import { get, post } from './http'

export interface SchemaColumnChange {
  table: string
  column: string
  expected?: string
  actual?: string
  reason?: string
}

export interface SchemaIndexChange {
  table: string
  name: string
  columns?: string
  unique?: boolean
}

export interface SchemaReport {
  checked_at?: string
  required: boolean
  schema_version: string
  existing?: boolean
  missing_core?: boolean
  has_super_admin?: boolean
  database_version?: string
  has_version_table?: boolean
  existing_tables?: string[]
  missing_tables?: string[]
  added_columns?: SchemaColumnChange[]
  modified_columns?: SchemaColumnChange[]
  added_indexes?: SchemaIndexChange[]
  extra_tables?: string[]
  extra_columns?: SchemaColumnChange[]
  warnings?: string[]
}

export interface SchemaStatus {
  required: boolean
  can_apply: boolean
  report: SchemaReport
}

export const schemaApi = {
  status: () => get<SchemaStatus>('/system/schema/status'),
  apply: () => post<{ applied: boolean; schema_version: string }>('/system/schema/apply'),
}
