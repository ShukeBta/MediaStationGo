import type { FormEvent, RefObject } from 'react'
import { Loader2, MessageSquare, Plus, Send, Trash2 } from 'lucide-react'

import type { AssistantMessage, AssistantSession, SessionView } from '../api/assistant'

export function AssistantSessionsSidebar({
  sessions,
  activeSessionId,
  loading,
  onNew,
  onSelect,
  onDelete,
}: {
  sessions: AssistantSession[]
  activeSessionId?: string
  loading: boolean
  onNew: () => void
  onSelect: (id: string) => void
  onDelete: (id: string) => void
}) {
  return (
    <aside className="glass-panel flex flex-col overflow-hidden">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-display text-sm font-semibold text-ink-600">会话</h2>
        <button onClick={onNew} className="neon-button !px-2 !py-1 text-xs">
          <Plus size={12} /> 新建
        </button>
      </div>
      {loading && (
        <div className="flex justify-center py-6 text-ink-50">
          <Loader2 className="animate-spin" />
        </div>
      )}
      <ul className="flex-1 space-y-1 overflow-y-auto pr-1">
        {sessions.map((session) => (
          <li
            key={session.id}
            className={
              'group flex items-center gap-2 rounded px-2 py-2 text-sm cursor-pointer ' +
              (activeSessionId === session.id
                ? 'bg-primary-400/10 text-brand-500'
                : 'text-ink-100 hover:bg-gray-50 hover:text-white')
            }
            onClick={() => onSelect(session.id)}
          >
            <MessageSquare size={14} className="shrink-0" />
            <span className="flex-1 truncate">{session.title || '未命名'}</span>
            <button
              onClick={(event) => {
                event.stopPropagation()
                onDelete(session.id)
              }}
              className="opacity-0 group-hover:opacity-100"
            >
              <Trash2 size={12} className="text-red-400" />
            </button>
          </li>
        ))}
        {!loading && sessions.length === 0 && (
          <li className="px-2 py-2 text-xs text-sand-500">暂无会话</li>
        )}
      </ul>
    </aside>
  )
}

export function AssistantConversationPane({
  active,
  draft,
  sending,
  messagesEndRef,
  onDraftChange,
  onSend,
}: {
  active: SessionView | null
  draft: string
  sending: boolean
  messagesEndRef: RefObject<HTMLDivElement>
  onDraftChange: (value: string) => void
  onSend: (event: FormEvent<HTMLFormElement>) => void
}) {
  return (
    <section className="glass-panel flex flex-col overflow-hidden">
      {!active && (
        <div className="m-auto text-center text-ink-50">
          <MessageSquare size={32} className="mx-auto mb-2 opacity-50" />
          <p>选择或创建一个会话开始对话</p>
        </div>
      )}
      {active && (
        <>
          <div className="mb-3 border-b border-gray-200 pb-2">
            <h2 className="font-display text-base font-semibold text-ink-600">
              {active.session.title || '未命名'}
            </h2>
          </div>
          <div className="flex-1 space-y-3 overflow-y-auto pr-2">
            {active.messages.length === 0 && (
              <p className="text-sm text-sand-500">说点什么开始对话…</p>
            )}
            {active.messages.map((message) => (
              <Bubble key={message.id} message={message} />
            ))}
            <div ref={messagesEndRef} />
          </div>
          <form onSubmit={onSend} className="mt-3 flex gap-2 border-t border-gray-200 pt-3">
            <input
              className="input-base flex-1"
              placeholder="发送消息…"
              value={draft}
              onChange={(event) => onDraftChange(event.target.value)}
              disabled={sending}
            />
            <button type="submit" disabled={sending || !draft.trim()} className="neon-button">
              {sending ? <Loader2 size={16} className="animate-spin" /> : <Send size={16} />}
              发送
            </button>
          </form>
        </>
      )}
    </section>
  )
}

function Bubble({ message }: { message: AssistantMessage }) {
  const mine = message.role === 'user'
  return (
    <div className={'flex ' + (mine ? 'justify-end' : 'justify-start')}>
      <div
        className={
          'max-w-[80%] whitespace-pre-wrap rounded-2xl px-4 py-2 text-sm ' +
          (mine
            ? 'bg-primary-400/20 text-primary-100'
            : message.role === 'system'
              ? 'border border-amber-400/30 bg-amber-400/5 text-amber-200'
              : 'bg-gray-50 text-ink-200')
        }
      >
        {message.content}
      </div>
    </div>
  )
}
