// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mysqllistalllocks

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/genai-toolbox/internal/embeddingmodels"
	"github.com/googleapis/genai-toolbox/internal/sources"
	"github.com/googleapis/genai-toolbox/internal/sources/cloudsqlmysql"
	"github.com/googleapis/genai-toolbox/internal/sources/mysql"
	"github.com/googleapis/genai-toolbox/internal/tools"
	"github.com/googleapis/genai-toolbox/internal/util"
	"github.com/googleapis/genai-toolbox/internal/util/parameters"
)

const resourceType string = "mysql-list-all-locks"

const listAllLocksStatement = `
SELECT
  dl.thread_id AS 'thread_id',
  it.TRX_MYSQL_THREAD_ID AS 'process_id',
  dl.object_schema AS 'db',
  dl.object_name AS 'table_name',
  dl.lock_type AS 'lock_type',
  dl.lock_mode AS 'lock_mode',
  dl.LOCK_STATUS AS 'lock_status',
  it.TRX_STATE AS 'transaction_state',
  it.TRX_OPERATION_STATE AS 'current_operation',
  substring(it.TRX_QUERY, 1, 100) AS 'query'
FROM
  performance_schema.data_locks dl
INNER JOIN information_schema.innodb_trx it
  ON dl.ENGINE_TRANSACTION_ID = it.TRX_ID
WHERE
  (dl.object_schema = COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) OR COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) IS NULL)
  AND (COALESCE(?, '') = '' OR dl.object_name = ?)
ORDER BY TRX_STARTED
LIMIT ?;
`

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	MySQLPool() *sql.DB
	RunSQL(context.Context, string, []any) (any, error)
}

type Config struct {
	Name         string   `yaml:"name" validate:"required"`
	Type         string   `yaml:"type" validate:"required"`
	Source       string   `yaml:"source" validate:"required"`
	Description  string   `yaml:"description" validate:"required"`
	AuthRequired []string `yaml:"authRequired"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func getConnectedSchema(cfg Config, srcs map[string]sources.Source) string {
	if src, ok := srcs[cfg.Source]; ok {
		switch mysqlSource := src.ToConfig().(type) {
		case mysql.Config:
			return mysqlSource.Database
		case cloudsqlmysql.Config:
			return mysqlSource.Database
		}
	}
	return ""
}

func (cfg Config) Initialize(srcs map[string]sources.Source) (tools.Tool, error) {
	allParameters := parameters.Parameters{
		parameters.NewStringParameterWithDefault("table_schema", "", "(Optional) The database where locked object is detected. Check all databases if not specified."),
		parameters.NewStringParameterWithDefault("table_name", "", "(Optional) Name of the table to be checked. Check all tables visible to the current user if not specified."),
		parameters.NewIntParameterWithDefault("limit", 10, "(Optional) Max rows to return, default is 10"),
		parameters.NewStringParameterWithDefault("connected_schema", getConnectedSchema(cfg, srcs), "(Optional) The connected db"),
	}
	mcpManifest := tools.GetMcpManifest(cfg.Name, cfg.Description, cfg.AuthRequired, allParameters, nil)

	// finish tool setup
	t := Tool{
		Config:      cfg,
		allParams:   allParameters,
		manifest:    tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
		mcpManifest: mcpManifest,
	}
	return t, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	Config
	allParams   parameters.Parameters `yaml:"parameters"`
	manifest    tools.Manifest
	mcpManifest tools.McpManifest
}

func (t Tool) Invoke(ctx context.Context, resourceMgr tools.SourceProvider, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, err := tools.GetCompatibleSource[compatibleSource](resourceMgr, t.Source, t.Name, t.Type)
	if err != nil {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, err)
	}

	paramsMap := params.AsMap()

	table_schema, ok := paramsMap["table_schema"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_schema' parameter; expected a string", nil)
	}
	table_name, ok := paramsMap["table_name"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_name' parameter; expected a string", nil)
	}
	limit, ok := paramsMap["limit"].(int)
	if !ok {
		return nil, util.NewAgentError("invalid 'limit' parameter; expected an integer", nil)
	}
	// Validate connected schema is either skipped or same as queried schema
	connected_schema := paramsMap["connected_schema"].(string)
	if table_schema != connected_schema && connected_schema != "" && table_schema != "" {
		err := fmt.Errorf("error: connected schema '%s' does not match queried schema '%s'", connected_schema, table_schema)
		return nil, util.NewClientServerError("schema match failed", http.StatusInternalServerError, err)
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, listAllLocksStatement))
	sliceParams := []any{table_schema, table_schema, table_name, table_name, limit}
	resp, err := source.RunSQL(ctx, listAllLocksStatement, sliceParams)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	return resp, nil
}

func (t Tool) EmbedParams(ctx context.Context, paramValues parameters.ParamValues, embeddingModelsMap map[string]embeddingmodels.EmbeddingModel) (parameters.ParamValues, error) {
	return parameters.EmbedParams(ctx, t.allParams, paramValues, embeddingModelsMap, nil)
}

func (t Tool) Manifest() tools.Manifest {
	return t.manifest
}

func (t Tool) McpManifest() tools.McpManifest {
	return t.mcpManifest
}

func (t Tool) Authorized(verifiedAuthServices []string) bool {
	return tools.IsAuthorized(t.AuthRequired, verifiedAuthServices)
}

func (t Tool) RequiresClientAuthorization(resourceMgr tools.SourceProvider) (bool, error) {
	return false, nil
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Config
}

func (t Tool) GetAuthTokenHeaderName(resourceMgr tools.SourceProvider) (string, error) {
	return "Authorization", nil
}

func (t Tool) GetParameters() parameters.Parameters {
	return t.allParams
}
