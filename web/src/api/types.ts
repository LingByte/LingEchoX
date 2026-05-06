/** Shared API list wrappers */
export interface Paginated<T> {
  list: T[]
  total: number
  page: number
  size: number
}
