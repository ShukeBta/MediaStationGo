import { createRoot } from 'react-dom/client'

import { PasswordDialog, type PasswordOptions } from './PasswordDialog'

export function requestPassword(options: PasswordOptions): Promise<string | null> {
  return new Promise((resolve) => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const close = (value: string | null) => {
      root.unmount()
      host.remove()
      resolve(value)
    }
    root.render(<PasswordDialog options={options} onClose={close} />)
  })
}
