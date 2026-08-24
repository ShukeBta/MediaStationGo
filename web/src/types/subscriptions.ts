import type { Media } from './media'

export interface Subscription {
  id: string
  user_id: string
  name: string
  feed_url: string
  delivery_mode: 'download' | 'resource_import'
  library_id?: string
  library_root_id?: string
  resource_source?: string
  max_imports_per_run?: number
  poll_interval_minutes?: number
  season_number?: number
  filter: string
  media_type?: string
  media_category?: string
  save_path?: string
  search_mode?: string
  imdb_id?: string
  source?: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  original_name?: string
  year?: number
  resolution?: string
  quality?: string
  effects?: string
  release_groups?: string
  exclude_words?: string
  min_seeders?: number
  max_seeders?: number
  min_size_gb?: number
  max_size_gb?: number
  free_only?: boolean
  wash_enabled?: boolean
  wash_priority?: string
  total_episodes?: number
  downloaded_episodes?: number
  local_media_count?: number
  missing_episodes?: number[]
  in_library?: boolean
  media_id?: string
  media?: Media
  series_key?: string
  priority?: number
  enabled: boolean
  last_run_at?: string
  catch_up_active?: boolean
  archived_at?: string
  archive_reason?: string
  import_jobs?: SubscriptionImportJob[]
  history_ids?: string[]
  created_at: string
  updated_at: string
}

export interface SubscriptionImportJob {
  id: string
  retry_of_job_id?: string
  attempt: number
  candidate_title?: string
  candidate_source?: string
  candidate_granularity?: string
  selected_episodes?: number[]
  moved_episodes?: number[]
  verified_episodes?: number[]
  scan_added?: number
  block_reason?: string
  status: string
  stage?: string
  outcome?: string
  error?: string
  created_at: string
  updated_at: string
  finished_at?: string
}
