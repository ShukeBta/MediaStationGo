import { api } from './client'

export interface PublicUIConfig {
  hide_community_links_for_users: boolean
}

export const publicAPI = {
  uiConfig: () => api.get<PublicUIConfig>('/public/ui-config').then((response) => response.data),
}
