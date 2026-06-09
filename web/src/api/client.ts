import type {
  ArchiveFilter,
  ArchiveResponse,
  Item,
  Rating,
  Source,
  TodayResponse,
  Topic,
} from './types'

// Empty base = same-origin: the Vite dev server proxies /api to the backend,
// and in production nginx proxies /api to the centrifuge container.
const BASE = import.meta.env.VITE_API_BASE || ''

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(BASE + path, {
      headers: { 'Content-Type': 'application/json' },
      ...init,
    })
  } catch (e) {
    throw new ApiError(0, `network error: ${(e as Error).message}`)
  }
  if (!res.ok) {
    let msg = res.statusText
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) msg = body.error
    } catch (_) {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function qs(filter: ArchiveFilter): string {
  const p = new URLSearchParams()
  if (filter.topic) p.set('topic', filter.topic)
  if (filter.source) p.set('source', filter.source)
  if (filter.q) p.set('q', filter.q)
  if (filter.from) p.set('from', filter.from)
  if (filter.to) p.set('to', filter.to)
  if (filter.limit != null) p.set('limit', String(filter.limit))
  if (filter.offset != null) p.set('offset', String(filter.offset))
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  today: (brief = false) =>
    request<TodayResponse>(`/api/today${brief ? '?brief=1' : ''}`),

  markSeen: () =>
    // keepalive lets this complete even as the page is unloading (pagehide).
    request<{ last_viewed_at: string }>('/api/today/seen', { method: 'POST', keepalive: true }),

  archive: (filter: ArchiveFilter = {}) =>
    request<ArchiveResponse>(`/api/archive${qs(filter)}`),

  item: (id: string) => request<Item>(`/api/items/${id}`),

  bookmark: (id: string) =>
    request<{ id: string; bookmarked: boolean }>(`/api/items/${id}/bookmark`, {
      method: 'POST',
    }),

  rate: (id: string, rating: Rating) =>
    request<{ id: string; rating: Rating }>(`/api/items/${id}/rate`, {
      method: 'POST',
      body: JSON.stringify({ rating }),
    }),

  markAd: (id: string) =>
    request<{ id: string; kind: string }>(`/api/items/${id}/mark-ad`, {
      method: 'POST',
    }),

  topics: () => request<{ topics: Topic[] }>('/api/topics'),

  sources: () => request<{ sources: Source[] }>('/api/sources'),
}
