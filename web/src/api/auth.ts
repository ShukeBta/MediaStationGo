import { api } from './client'
import type { User, TokenPair } from '../types'

export interface LoginResponse {
  user: User
  tokens: TokenPair
}

export const authAPI = {
  login: (username: string, password: string) =>
    api.post<LoginResponse>('/auth/login', { username, password }).then((r) => r.data),

  me: () => api.get<User>('/me').then((r) => r.data),

  changePassword: (oldPassword: string, newPassword: string) =>
    api
      .post('/me/password', { old_password: oldPassword, new_password: newPassword })
      .then((r) => r.data),
}
