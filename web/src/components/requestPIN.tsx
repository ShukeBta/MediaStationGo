import { createRoot } from 'react-dom/client'

import { PinDialog, type PinOptions } from './PinDialog'

export function requestPIN(options: PinOptions): Promise<string | null> {
  return new Promise((resolve) => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    const close = (value: string | null) => {
      root.unmount()
      host.remove()
      resolve(value)
    }
    root.render(<PinDialog options={options} onClose={close} />)
  })
}
