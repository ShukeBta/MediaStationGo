export interface Media {
  id: string
  library_id: string
  library_root_id?: string
  library_name?: string
  library_path?: string
  display_library_id?: string
  display_library_name?: string
  display_library_path?: string
  series_id?: string
  title: string
  original_name?: string
  episode_title?: string
  path: string
  relative_path?: string
  size_bytes: number
  duration_sec: number
  width: number
  height: number
  video_codec?: string
  audio_codec?: string
  container?: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  rating: number
  year: number
  release_date?: string
  season_num: number
  episode_num: number
  scrape_status: string
  tmdb_id: number
  bangumi_id: number
  douban_id?: string
  thetvdb_id?: string
  languages?: string
  countries?: string
  genres?: string
  nsfw: boolean
  strm_url?: string
  file_hash?: string
  file_id?: string
  is_duplicate?: boolean
  duplicate_of?: string
  versions?: Media[]
  created_at: string
  updated_at: string
}

export interface Playlist {
  id: string
  user_id: string
  name: string
  is_public: boolean
  created_at: string
  updated_at: string
}
