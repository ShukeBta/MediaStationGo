export interface PlayProfile {
  id: string
  user_id: string
  name: string
  is_default: boolean
  content_rating_limit?: string
  allow_adult: boolean
  require_pin: boolean
  preferred_subtitle_lang?: string
  preferred_audio_lang?: string
  autoplay_next: boolean
  skip_intro: boolean
  allowed_library_ids: string[]
  total_watch_time: number
  last_active_at?: string
  created_at: string
  updated_at: string
}
