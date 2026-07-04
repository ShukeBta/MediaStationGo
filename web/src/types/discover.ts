export interface DiscoverSection {
  key: string
  label: string
}

export interface DiscoverItem {
  TMDbID?: number
  Title?: string
  Overview?: string
  Rating?: number
  Year?: number
  PosterURL?: string
  BackdropURL?: string
  // Match struct (Go) is exported with capitalised JSON keys; the API
  // returns lower-cased aliases below for convenience.
  tmdb_id?: number
  title?: string
  overview?: string
  rating?: number
  year?: number
  poster_url?: string
  backdrop_url?: string
}
