import { useState } from 'react'
import toast from 'react-hot-toast'

import type { DiscoverItem } from '../api/discover'
import { subscriptionsAPI } from '../api/subscriptions'
import { discoverItemSource } from './discoverPageModel'
import {
  DiscoverArtworkPanel,
  DiscoverModalHeader,
  DiscoverOverviewPanel,
  DiscoverSubscriptionRules,
} from './DiscoverDetailModalSections'
import {
  apiErrorMessage,
  buildDiscoverSubscriptionInput,
  initialDiscoverSubscriptionForm,
} from './discoverDetailModalModel'

export function DiscoverDetailModal({ item, onClose }: { item: DiscoverItem; onClose: () => void }) {
  const source = discoverItemSource(item)
  const [form, setForm] = useState(() => initialDiscoverSubscriptionForm(item))
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    setBusy(true)
    try {
      const sub = await subscriptionsAPI.create(buildDiscoverSubscriptionInput(item, form, source))
      if (form.run_now) {
        const run = await subscriptionsAPI.runNow(sub.id)
        toast.success(run.queued > 0 ? `已订阅并加入 ${run.queued} 个下载` : '已订阅，暂未命中可下载资源')
      } else {
        toast.success('已创建订阅')
      }
      onClose()
    } catch (err) {
      toast.error(apiErrorMessage(err, '订阅失败'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4 backdrop-blur-sm">
      <div className="max-h-[92vh] w-full max-w-5xl overflow-y-auto rounded-3xl border border-white/60 bg-white p-5 shadow-2xl">
        <DiscoverModalHeader item={item} source={source} onClose={onClose} />
        <div className="grid gap-5 lg:grid-cols-[260px_1fr]">
          <DiscoverArtworkPanel item={item} />
          <div className="space-y-5">
            <DiscoverOverviewPanel overview={item.overview} />
            <DiscoverSubscriptionRules
              form={form}
              busy={busy}
              onChange={(patch) => setForm((current) => ({ ...current, ...patch }))}
              onSubmit={submit}
            />
          </div>
        </div>
      </div>
    </div>
  )
}
