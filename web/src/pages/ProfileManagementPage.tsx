import { useEffect, useState } from 'react'
import { Loader2, Plus, UserCog } from 'lucide-react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import { playProfilesAPI } from '../api/play_profiles'
import { useAuthStore } from '../stores/auth'
import { usePlayProfileStore } from '../stores/playProfile'
import { confirmAction } from '../components/confirmAction'
import { requestPassword } from '../components/requestPassword'
import { requestPIN } from '../components/requestPIN'
import type { Library, PlayProfile } from '../types'
import { ProfileCard } from './ProfileCard'
import { ProfileFormModal } from './ProfileFormModal'

const MAX_PLAY_PROFILES = 3

// ProfileManagementPage replicates the Vue ProfileManagementView. It
// lets each user define private "viewing personas" with
// different content-rating gates, library access, and player defaults.
//
// All persistence is real: data is written to /api/play-profiles which
// is backed by the Go PlayProfileService.
export function ProfileManagementPage() {
  const userID = useAuthStore((s) => s.user?.id ?? '')
  const activeProfileId = usePlayProfileStore((s) => s.activeProfileId)
  const setActiveProfile = usePlayProfileStore((s) => s.setActiveProfile)

  const [profiles, setProfiles] = useState<PlayProfile[]>([])
  const [libraries, setLibraries] = useState<Library[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<PlayProfile | null>(null)
  const [showForm, setShowForm] = useState(false)

  const refresh = async () => {
    setLoading(true)
    try {
      const [p, l] = await Promise.all([
        playProfilesAPI.list(),
        libraryAPI.list().catch(() => [] as Library[]),
      ])
      setProfiles(p)
      setLibraries(l)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onDelete = async (p: PlayProfile) => {
    if (!(await confirmAction({ title: '删除播放档案', message: `确定删除 Profile「${p.name}」? 删除前需要再次验证。`, confirmText: '继续删除' }))) return
    try {
      const proof: { pin?: string; password?: string } = {}
      if (p.require_pin) {
        const pin = await requestPIN({
          title: '删除 Profile 需要 PIN',
          message: `请输入「${p.name}」的 PIN；也可以取消后改用账号密码删除。`,
          profileName: p.name,
        })
        if (!pin) return
        proof.pin = pin
      } else {
        const password = await requestPassword({
          title: '删除 Profile 需要密码',
          message: `请输入当前账号密码以删除「${p.name}」。`,
          confirmText: '删除',
        })
        if (!password) return
        proof.password = password
      }
      await playProfilesAPI.remove(p.id, proof)
      toast.success('已删除')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '删除失败'
      toast.error(msg)
    }
  }

  const openCreate = () => {
    if (profiles.length >= MAX_PLAY_PROFILES) {
      toast.error(`每个用户最多只能创建 ${MAX_PLAY_PROFILES} 个观影 Profile`)
      return
    }
    setEditing(null)
    setShowForm(true)
  }

  const openEdit = (p: PlayProfile) => {
    setEditing(p)
    setShowForm(true)
  }

  const selectProfile = async (profile: PlayProfile) => {
    if (profile.user_id !== userID) {
      toast.error('只能切换当前账号自己的 Profile')
      return
    }
    try {
      let pinToken: string | null = null
      if (profile.require_pin) {
        const pin = await requestPIN({ profileName: profile.name })
        if (!pin) return
        const verified = await playProfilesAPI.verifyPin(profile.id, pin)
        pinToken = verified.token
      }
      setActiveProfile(profile.id, pinToken)
      toast.success(`已切换到「${profile.name}」`)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'PIN 验证失败'
      toast.error(msg)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-purple-400/10 text-purple-400">
            <UserCog size={20} />
          </div>
          <div>
            <h1 className="font-display text-3xl font-bold text-ink-600">观影 Profile</h1>
            <p className="text-sm text-ink-50">
              为不同场景(儿童 / 影院 / 成人)定义独立的内容分级和媒体库访问规则
            </p>
          </div>
        </div>
        <button
          onClick={openCreate}
          disabled={profiles.length >= MAX_PLAY_PROFILES}
          className="neon-button disabled:cursor-not-allowed disabled:opacity-50"
          title={`每个用户最多 ${MAX_PLAY_PROFILES} 个 Profile`}
        >
          <Plus size={16} /> 创建 Profile
        </button>
      </div>
      <div className="rounded-2xl border border-primary-400/15 bg-primary-400/5 px-4 py-3 text-sm text-ink-100">
        当前账号已创建 {profiles.length}/{MAX_PLAY_PROFILES} 个 Profile。Profile 仅当前用户可见，不会与其他用户共享。
      </div>

      {loading && (
        <div className="flex justify-center py-12 text-ink-50">
          <Loader2 className="animate-spin" />
        </div>
      )}

      {!loading && profiles.length === 0 && (
        <div className="glass-panel py-12 text-center">
          <div className="mb-2 text-4xl">👤</div>
          <p className="font-medium text-ink-600">暂无 Profile</p>
          <p className="text-sm text-ink-50">点击右上角"创建 Profile"开始</p>
        </div>
      )}

      {!loading && profiles.length > 0 && (
        <div className="grid gap-3">
          {profiles.map((p) => (
            <ProfileCard
              key={p.id}
              profile={p}
              libraries={libraries}
              active={activeProfileId === p.id || (!activeProfileId && p.is_default)}
              onSelect={() => selectProfile(p)}
              onEdit={() => openEdit(p)}
              onDelete={() => onDelete(p)}
            />
          ))}
        </div>
      )}

      {showForm && (
        <ProfileFormModal
          editing={editing}
          libraries={libraries}
          defaultUserID={userID}
          onClose={() => setShowForm(false)}
          onSaved={async () => {
            setShowForm(false)
            await refresh()
          }}
        />
      )}
    </div>
  )
}
