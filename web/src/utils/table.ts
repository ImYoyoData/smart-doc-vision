export type TableSortOrder = 'ascend' | 'descend' | false

export interface TableSorterState {
  columnKey?: string | number
  order?: TableSortOrder
}

// Converts Naive DataTable sorter state into a stable `field:asc|desc,...`
// query string. Naive keeps multiple sorters in click order, so the first
// entry becomes the primary sort field.
export function sortersToQuery(value: unknown): string {
  const list = (Array.isArray(value) ? value : value ? [value] : []) as TableSorterState[]
  return list
    .filter((item) => item.columnKey !== undefined && (item.order === 'ascend' || item.order === 'descend'))
    .map((item) => `${String(item.columnKey)}:${item.order === 'ascend' ? 'asc' : 'desc'}`)
    .join(',')
}
