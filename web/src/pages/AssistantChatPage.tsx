import { FormEvent, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'

import {
  assistantAPI,
  type AssistantMessage,
  type AssistantSession,
  type SessionView,
} from '../api/assistant'
import { confirmAction } from '../components/confirmAction'
import { AssistantConversationPane, AssistantSessionsSidebar } from './AssistantChatSections'

// AssistantChatPage is the multi-turn chat surface backed by the Go
// AssistantService. It complements the older AIAssistantPage which is
// limited to single-turn smart search + recommendations.
export function AssistantChatPage() {
  const [sessions, setSessions] = useState<AssistantSession[]>([])
  const [active, setActive] = useState<SessionView | null>(null)
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [loading, setLoading] = useState(true)
  const messagesEndRef = useRef<HTMLDivElement | null>(null)

  const refreshSessions = async () => {
    try {
      const list = await assistantAPI.listSessions()
      setSessions(list)
      // Auto-select the most recent if nothing is open.
      if (list.length > 0 && !active) {
        const view = await assistantAPI.getSession(list[0].id)
        setActive(view)
      }
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refreshSessions().catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [active?.messages.length])

  const onNew = async () => {
    try {
      const sess = await assistantAPI.createSession('New chat')
      const view = await assistantAPI.getSession(sess.id)
      setActive(view)
      await refreshSessions()
    } catch {
      toast.error('创建会话失败')
    }
  }

  const onSelect = async (id: string) => {
    try {
      setActive(await assistantAPI.getSession(id))
    } catch {
      toast.error('加载会话失败')
    }
  }

  const onDelete = async (id: string) => {
    if (!(await confirmAction({ title: '删除会话', message: '删除此会话?', confirmText: '删除' }))) return
    try {
      await assistantAPI.deleteSession(id)
      if (active?.session.id === id) setActive(null)
      await refreshSessions()
    } catch {
      toast.error('删除失败')
    }
  }

  const onSend = async (e: FormEvent) => {
    e.preventDefault()
    if (!draft.trim() || !active) return
    setSending(true)
    const text = draft.trim()
    setDraft('')
    // Optimistic append so the user's turn shows immediately.
    setActive((s) =>
      s
        ? {
            ...s,
            messages: [
              ...s.messages,
              {
                id: 'pending-' + Date.now(),
                session_id: s.session.id,
                role: 'user',
                content: text,
                created_at: new Date().toISOString(),
              } as AssistantMessage,
            ],
          }
        : s,
    )
    try {
      const view = await assistantAPI.chat(active.session.id, text)
      setActive(view)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '发送失败'
      toast.error(msg)
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="grid h-[calc(100vh-100px)] grid-cols-[260px_1fr] gap-4">
      <AssistantSessionsSidebar
        sessions={sessions}
        activeSessionId={active?.session.id}
        loading={loading}
        onNew={onNew}
        onSelect={onSelect}
        onDelete={onDelete}
      />
      <AssistantConversationPane
        active={active}
        draft={draft}
        sending={sending}
        messagesEndRef={messagesEndRef}
        onDraftChange={setDraft}
        onSend={onSend}
      />
    </div>
  )
}
