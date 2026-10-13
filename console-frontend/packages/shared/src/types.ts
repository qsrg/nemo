/** 公共类型定义（架构 §8.5 packages/types 职责，demo 收敛于 shared）。 */

export interface User {
  id: string
  username: string
  name: string
  email?: string
}

export interface Order {
  id: string
  userId: string
  userName?: string
  sku: string
  quantity: number
  amount: number
  status: string
  createdAt: string
}

export interface PagedResult<T> {
  items: T[]
  total: number
}

export interface LoginResult {
  token: string
  user: User
}
