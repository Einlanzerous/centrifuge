// Mirrors the Go DTOs in internal/httpapi (CTFG-26). Keep in sync with views.go.

export type Rating = 'up' | 'down' | 'none'

/** A scored story as the UI consumes it (httpapi.itemDTO). */
export interface Item {
  id: string
  title: string
  summary: string
  snippet?: string
  url?: string
  primary_topic: string
  /** Stable HSL assigned server-side from the topic name (palette.go). */
  topic_color: string
  /** Hero image for the card/reader art header; absent → topic gradient. */
  image_url?: string
  labels: string[]
  section?: string
  kind: string
  relevance_score?: number
  source_id: string
  source_name: string
  received: string // RFC3339
  bookmarked: boolean
  rating: Rating
  read: boolean
  /** Parent newsletter raw HTML — only populated by GET /api/items/{id}. */
  body?: string
  /** True when body is a multi-story digest (not this single story). */
  segmented?: boolean
  /** This story's verbatim text, sliced from the newsletter (digest items). */
  content?: string
}

/** One topic-registry / chip entry (httpapi.topicDTO). */
export interface Topic {
  topic: string
  color: string
  count: number
}

/** One source-registry entry (httpapi.sourceDTO). */
export interface Source {
  id: string
  name: string
  story_count: number
  scored_count: number
  avg_relevance: number | null
  bookmark_count: number
  positive_ratings: number
  negative_ratings: number
}

export interface TodayResponse {
  since: string | null
  since_human: string
  is_empty: boolean
  brief: boolean
  topics: Topic[]
  items: Item[]
}

export interface DayGroup {
  date: string // YYYY-MM-DD
  label: string // "Today" / "Yesterday" / weekday
  items: Item[]
}

export interface ArchiveResponse {
  days: DayGroup[]
  total: number
  sources: Source[]
  topics: Topic[]
}

/** Server-side archive query params (archive.go). */
export interface ArchiveFilter {
  topic?: string
  source?: string
  q?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}
