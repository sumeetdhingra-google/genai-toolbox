---
title: "mysql-list-table-stats"
type: docs
weight: 1
description: >
  A "mysql-list-table-stats" tool report table statistics including table size, total latency, rows read, rows written, read and write latency for entire instance, a specified database, or a specified table.
aliases:
- /resources/tools/mysql-list-table-stats
---

## About

A `mysql-list-table-stats` tool checks table size, estimated rows count, total latency, rows fetched, rows inserted, rows udpdated, rows deleted, number of IO reads and IO latency, number of IO writes and IO write latency, number of IO misc operations and IO misc latency.
IO latency of reads and writes on table by querying the tables and table statitics in sys schema.

Compatible sources:
- [cloud-sql-mysql](../../sources/cloud-sql-mysql.md)
- [mysql](../../sources/mysql.md)

`mysql-list-table-stats` outputs detailed information about row counts, total latency and reads and writes on table since the MySQL instance was restarted as JSON. Results are sorted by total latency in secs in descreasing order and are limited to 10 rows.
This tool takes 4 optional input parameters:

- `table_schema` (optional): The database where fragmentation check is to be
  executed. Check all tables visible to the current user if not specified.
- `table_name` (optional): Name of the table to be checked. Check all tables
  visible to the current user if not specified.
- `sort_by` (optional): The column to sort by. Valid values are `row_count`, `rows_fetched`, `rows_inserted`, `rows_updated`, `rows_deleted`, `total_latency_secs` (defaults to `total_latency_secs`)
- `limit` (optional): Max rows to return, default 10.

## Example

```yaml
kind: tools
name: list_table_stats
type: mysql-list-table-fragmentation
source: my-mysql-instance
description: Display table statistics including table size, total latency, rows read, rows written, read and write latency for entire instance, a specified database, or a specified table. Specifying a database name or table name filters the output to that specific db or table. Results are limited to 50 by default, with a maximum allowable limit of 1000.
```

The response is a json array with the following fields:

```json
{
  "table_schema": "The schema/database this table belongs to",
  "table_name": "Name of this table",
  "size_MB": "Size of the table data in MB",
  "row_count": "Number of rows in the table",
  "total_latency_secs": "total latency in secs",
  "rows_fetched": "total number of rows fetched",
  "rows_inserted": "total number of rows inserted",
  "rows_updated": "total number of rows updated",
  "rows_deleted": "total number of rows deleted",
  "io_reads": "total number of io read requests",
  "io_read_latency": "io read latency in seconds",
  "io_write_latency": "io write latency in seconds",
  "io_misc_latency": "io misc latency in seconds",
}
```

## prerequisite

- `performance_schema` should be turned ON for this tool to work. 

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "mysql-list-table-stats".                  |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |


## Use Cases

- **Finding hottest tables**: Identify tables with highest total latency, read or writes based on the `sort_by` column. 
- **Finding tables with most reads**: Identify tables with highest reads by sorting on `rows_fetched`. 
- **Monitoring growth**: Track `row_count` and `size_MB` of table over time to estimate growth.

