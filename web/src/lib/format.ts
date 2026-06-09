// Relative + absolute time formatting for received timestamps (RFC3339 strings).
//
// The relative scale tops out at a 3-character value token so it can stack
// cleanly in narrow columns: hours are the smallest unit we surface, and the
// units climb h → d → m (months) → y. Sub-hour reads as "now".

export interface RelParts {
  /** ≤3-char value token: "now" | "14h" | "30d" | "11m" | "2y". */
  token: string
  /** Whether an "ago" suffix applies (false for "now"). */
  ago: boolean
}

export function relTime(iso: string, ref: Date = new Date()): RelParts {
  const h = Math.floor(Math.max(0, ref.getTime() - new Date(iso).getTime()) / 3_600_000)
  if (h < 1) return { token: 'now', ago: false }
  if (h < 24) return { token: `${h}h`, ago: true }
  const d = Math.floor(h / 24)
  const y = Math.floor(d / 365)
  if (y >= 1) return { token: `${y}y`, ago: true }
  const m = Math.floor(d / 30)
  if (m >= 1) return { token: `${m}m`, ago: true }
  return { token: `${d}d`, ago: true }
}

/** Inline one-line form, e.g. "14h ago" / "now". */
export function timeAgo(iso: string, ref: Date = new Date()): string {
  const p = relTime(iso, ref)
  return p.ago ? `${p.token} ago` : p.token
}

export function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}
