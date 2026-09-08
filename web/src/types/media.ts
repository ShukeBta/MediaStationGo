export interface Media {
  id: string
  library_id: string
  library_root_id?: string
  library_name?: string
  library_path?: string
  display_library_id?: string
  display_library_name?: string
  display_library_path?: string
  auto_category?: string
  adult_type?: 'AV' | 'FC2'
  series_id?: string
  title: string
  display_title?: string
  original_name?: string
  episode_title?: string
  part_group_key?: string
  part_group_title?: string
  part_index?: number
  version_group_key?: string
  title_cleanup_version?: number
  path?: string
  relative_path?: string
  size_bytes: number
  duration_sec: number
  width: number
  height: number
  video_codec?: string
  audio_codec?: string
  container?: string
  bit_rate?: number
  video_bit_rate?: number
  frame_rate?: number
  video_profile?: string
  video_range?: string
  video_bit_depth?: number
  audio_bit_rate?: number
  audio_channels?: number
  audio_channel_layout?: string
  audio_sample_rate?: number
  media_probe_version?: number
  poster_url?: string
  backdrop_url?: string
  generated_poster_url?: string
  generated_backdrop_url?: string
  generated_artwork_seek_sec?: number
  overview?: string
  rating: number
  year: number
  release_date?: string
  season_num: number
	 episode_end_num?: number
	 episode_part_num?: number
  episode_num: number
  scrape_status: string
  tmdb_id: number
  bangumi_id: number
  douban_id?: string
  thetvdb_id?: string
  languages?: string
  countries?: string
  genres?: string
  actors?: string
  nsfw: boolean
  strm_url?: string
  file_hash?: string
  file_id?: string
  is_duplicate?: boolean
  duplicate_of?: string
  versions?: Media[]
  parts?: Media[]
  created_at: string
  updated_at: string
}

export interface MediaVersion extends Media {
  can_manage: boolean
  is_current: boolean
}

export interface MediaVersionList {
  items: MediaVersion[]
  can_manage_versions: boolean
}

export interface MediaPart extends Media {
  is_current: boolean
}

export interface MediaPartList {
  items: MediaPart[]
}

export interface Playlist {
  id: string
  user_id: string
  name: string
  is_public: boolean
  created_at: string
  updated_at: string
}
