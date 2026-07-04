import { EVENT_OPTIONS, type EventMode } from './notifyChannelsModel'
import { Field } from './NotifyChannelFormField'

type NotifyChannelEventFieldsProps = {
  events: string[]
  eventMode: EventMode
  setEventMode: (mode: EventMode) => void
  toggleEvent: (event: string) => void
}

export function NotifyChannelEventFields({
  events,
  eventMode,
  setEventMode,
  toggleEvent,
}: NotifyChannelEventFieldsProps) {
  return (
    <Field label="推送事件">
      <div className="space-y-2 rounded-lg border border-gray-200 bg-gray-50/60 p-3">
        <label className="flex cursor-pointer items-center gap-2 text-sm text-ink-100">
          <input
            type="radio"
            name="notify-event-mode"
            className="h-4 w-4 accent-primary-400"
            checked={eventMode === 'all'}
            onChange={() => setEventMode('all')}
          />
          全部事件
        </label>
        <label className="flex cursor-pointer items-center gap-2 text-sm text-ink-100">
          <input
            type="radio"
            name="notify-event-mode"
            className="h-4 w-4 accent-primary-400"
            checked={eventMode === 'none'}
            onChange={() => setEventMode('none')}
          />
          关闭全部推送事件
        </label>
        <label className="flex cursor-pointer items-center gap-2 text-sm text-ink-100">
          <input
            type="radio"
            name="notify-event-mode"
            className="h-4 w-4 accent-primary-400"
            checked={eventMode === 'custom'}
            onChange={() => setEventMode('custom')}
          />
          仅推送勾选事件
        </label>
        <div className="grid gap-2 sm:grid-cols-2">
          {EVENT_OPTIONS.map((event) => (
            <label key={event.value} className="flex cursor-pointer items-center gap-2 text-sm text-ink-100">
              <input
                type="checkbox"
                className="h-4 w-4 accent-primary-400"
                disabled={eventMode !== 'custom'}
                checked={events.includes(event.value)}
                onChange={() => toggleEvent(event.value)}
              />
              {event.label}
            </label>
          ))}
        </div>
      </div>
    </Field>
  )
}
