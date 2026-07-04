import { api } from './client'

// ── License Server types (generic, ready for MediaStationGo license integration) ──

/** Response after binding/activating a license. */
export interface LicenseActivation {
  id: string
  key_id: string
  /** The license key string (e.g. MS-XXXX-XXXX-XXXX) */
  key?: string
  device_id: string
  device_name?: string
  plan?: string
  max_activations?: number
  max_users?: number | null
  unlimited_users?: boolean
  /** ISO8601 — null means perpetual */
  expires_at?: string | null
  valid: boolean
  ip?: string
  heartbeat_at?: string | null
  created_at: string
}

/** Status of the currently bound license on this device. */
export interface LicenseStatus {
  /** Whether a license is currently active */
  active: boolean
  activation?: LicenseActivation
  max_users?: number | null
  unlimited_users?: boolean
  /** Error or status message */
  message?: string
}

// ── API methods ──

export const licenseAPI = {
  /** Bind / activate a license key for this device. */
  bind: (key: string) =>
    api
      .post<LicenseActivation>('/license/activate', {
        key: key.trim(),
      })
      .then((r) => r.data),

  /** Get the status of the currently active license. */
  status: () =>
    api.get<LicenseStatus>('/license/status').then((r) => r.data),

  /** Refresh the heartbeat for the active license. */
  heartbeat: () =>
    api.post('/license/heartbeat').then((r) => r.data),
}
