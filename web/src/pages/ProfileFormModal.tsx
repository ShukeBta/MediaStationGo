import { FormEvent, useState } from 'react'
import toast from 'react-hot-toast'

import { playProfilesAPI, type PlayProfileInput } from '../api/play_profiles'
import type { Library, PlayProfile } from '../types'
import {
  ProfileFormActions,
  ProfileIdentityFields,
  ProfileLibraryAccessField,
  ProfilePreferenceFields,
} from './ProfileFormSections'

type ProfileFormModalProps = {
  editing: PlayProfile | null
  libraries: Library[]
  defaultUserID: string
  onClose: () => void
  onSaved: () => void | Promise<void>
}

export function ProfileFormModal({
  editing,
  libraries,
  defaultUserID,
  onClose,
  onSaved,
}: ProfileFormModalProps) {
  const [form, setForm] = useState<PlayProfileInput>(() => ({
    user_id: editing?.user_id ?? defaultUserID,
    name: editing?.name ?? '',
    is_default: editing?.is_default ?? false,
    content_rating_limit: editing?.content_rating_limit ?? '',
    allow_adult: editing?.allow_adult ?? false,
    require_pin: editing?.require_pin ?? false,
    pin: '',
    preferred_subtitle_lang: editing?.preferred_subtitle_lang ?? '',
    preferred_audio_lang: editing?.preferred_audio_lang ?? '',
    autoplay_next: editing?.autoplay_next ?? true,
    skip_intro: editing?.skip_intro ?? false,
    allowed_library_ids: editing?.allowed_library_ids ?? [],
  }))
  const [saving, setSaving] = useState(false)

  const update = (patch: Partial<PlayProfileInput>) => setForm((current) => ({ ...current, ...patch }))

  const toggleLib = (id: string) => {
    const next = form.allowed_library_ids.includes(id)
      ? form.allowed_library_ids.filter((item) => item !== id)
      : [...form.allowed_library_ids, id]
    update({ allowed_library_ids: next })
  }

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    setSaving(true)
    try {
      if (editing) {
        await playProfilesAPI.update(editing.id, form)
      } else {
        await playProfilesAPI.create(form)
      }
      toast.success('已保存')
      await onSaved()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="glass-panel w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <h2 className="mb-4 font-display text-xl font-semibold text-ink-600">
          {editing ? '编辑 Profile' : '创建 Profile'}
        </h2>
        <form onSubmit={onSubmit} className="space-y-4">
          <ProfileIdentityFields form={form} editing={editing} update={update} />
          <ProfilePreferenceFields form={form} update={update} />
          <ProfileLibraryAccessField form={form} libraries={libraries} onToggleLibrary={toggleLib} />
          <ProfileFormActions saving={saving} onClose={onClose} />
        </form>
      </div>
    </div>
  )
}
