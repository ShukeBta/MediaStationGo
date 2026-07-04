import { createRoot } from 'react-dom/client'

import { ConfirmDialog, type ConfirmOptions } from './ConfirmDialog'

export function confirmAction(options: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const close = (value: boolean) => {
      root.unmount()
      host.remove()
      resolve(value)
    }
    root.render(<ConfirmDialog options={options} onClose={close} />)
  })
}
