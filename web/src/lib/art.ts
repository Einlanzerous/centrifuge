// Turns a topic's server-assigned color into the visual language the design
// calls for — gradient "art" headers, accent dots, and pill tags — entirely
// from the topic's hue. Because the hue is derived server-side from the topic
// name (palette.go), a growing taxonomy keeps stable, distinct colors with no
// hardcoded topic list (the CTFG-28 guardrail).

/** Extract the hue from an `hsl(H, S%, L%)` string; falls back to a neutral. */
export function hueOf(topicColor: string | undefined): number {
  if (!topicColor) return 40
  const m = topicColor.match(/hsl\(\s*([\d.]+)/i)
  return m ? Math.round(parseFloat(m[1])) % 360 : 40
}

/** A layered gradient header for a topic, composed from its hue. */
export function artGradient(topicColor: string | undefined): string {
  const h = hueOf(topicColor)
  return [
    `radial-gradient(60% 50% at 22% 30%, hsl(${h} 90% 70%) 0%, transparent 60%)`,
    `radial-gradient(50% 60% at 80% 80%, hsl(${h} 78% 36%) 0%, transparent 55%)`,
    `conic-gradient(from 200deg at 50% 50%, hsl(${h} 70% 88%), hsl(${h} 75% 60%), hsl(${h} 80% 34%), hsl(${h} 70% 88%))`,
  ].join(', ')
}

/** A flat fallback color shown before/behind the gradient. */
export function artFallback(topicColor: string | undefined): string {
  return `hsl(${hueOf(topicColor)} 45% 60%)`
}

/** The saturated accent dot for a topic. */
export function dotColor(topicColor: string | undefined): string {
  return `hsl(${hueOf(topicColor)} 75% 55%)`
}

/** Inline style for a pill tag off a topic hue (light bg, deep fg). */
export function tagStyle(topicColor: string | undefined): Record<string, string> {
  const h = hueOf(topicColor)
  return { background: `hsl(${h} 70% 94%)`, color: `hsl(${h} 55% 30%)` }
}

/** A deterministic art height for masonry rhythm, derived from an id. */
const HEIGHTS = ['h-32', 'h-44', 'h-56', 'h-40', 'h-48']
export function artHeight(id: string): string {
  let sum = 0
  for (let i = 0; i < id.length; i++) sum = (sum + id.charCodeAt(i)) % 997
  return HEIGHTS[sum % HEIGHTS.length]
}
