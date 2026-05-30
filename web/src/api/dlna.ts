import { api } from './client'

export interface DLNADevice {
  udn: string
  friendly_name: string
  manufacturer: string
  model_name: string
  location: string
  control_url: string
  ip_address: string
}

export const dlnaAPI = {
  list: (force = false) =>
    api
      .get<{ devices: DLNADevice[] | null }>('/dlna/devices', { params: { force: force ? 'true' : '' } })
      .then((r) => r.data.devices ?? []),

  cast: (controlURL: string, mediaURL: string) =>
    api
      .post('/dlna/cast', { control_url: controlURL, media_url: mediaURL })
      .then((r) => r.data),
}
