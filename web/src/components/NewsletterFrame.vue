<script setup lang="ts">
import { ref, watch } from 'vue'
import { useTheme } from '../composables/useTheme'

// Renders raw newsletter HTML inside a sandboxed iframe. This isolates the
// email's styles from the app and, because `sandbox` omits `allow-scripts`,
// neutralizes any embedded JS (no XSS). `allow-same-origin` is kept so the
// parent can measure the document and toggle the theme override on it.
//
// Theme handling: the iframe loads the email once (srcdoc never changes). On
// load we measure whether the email paints light or dark, then add or remove an
// invert <style> directly in the loaded document so it matches the app theme —
// instantly, with no reload, in either direction. raw=true shows the email's own
// colors regardless. Already-dark emails in dark mode are left untouched.
const props = withDefaults(defineProps<{ html: string; raw?: boolean }>(), { raw: false })

const { dark } = useTheme()
const frame = ref<HTMLIFrameElement | null>(null)
const ready = ref(false)

let emailDark = false
let measured = false

const INVERT_CSS =
  'html{background:#ffffff!important;filter:invert(1) hue-rotate(180deg);}' +
  'img,picture,video,svg,canvas,[background],[style*="background-image"],' +
  '[style*="url("]{filter:invert(1) hue-rotate(180deg);}'

function frameDoc(): Document | null {
  try {
    return frame.value?.contentDocument ?? null
  } catch {
    return null
  }
}

function parseRGB(c: string): { r: number; g: number; b: number; a: number } | null {
  const m = c.match(/rgba?\(([^)]+)\)/i)
  if (!m) return null
  const p = m[1].split(',').map((s) => parseFloat(s.trim()))
  return { r: p[0], g: p[1], b: p[2], a: p[3] ?? 1 }
}

// Measure the email's predominant background luminance on the *raw* document
// (before any override is applied). Defaults to light when nothing opaque is
// found — most marketing emails are white.
function measureEmailDark(): boolean {
  const d = frameDoc()
  if (!d) return false
  const els: (Element | null)[] = [d.body, d.documentElement]
  if (d.body) els.push(...Array.from(d.body.children).slice(0, 4))
  for (const el of els) {
    if (!el) continue
    const rgb = parseRGB(getComputedStyle(el).backgroundColor)
    if (rgb && rgb.a > 0.5) {
      return (0.299 * rgb.r + 0.587 * rgb.g + 0.114 * rgb.b) / 255 < 0.4
    }
  }
  return false
}

// Add or remove the invert override in the live document — no reload.
function applyOverride() {
  const d = frameDoc()
  if (!d || !d.head) return
  const want = !props.raw && emailDark !== dark.value
  const existing = d.getElementById('__cf_invert')
  if (want && !existing) {
    const st = d.createElement('style')
    st.id = '__cf_invert'
    st.textContent = INVERT_CSS
    d.head.appendChild(st)
  } else if (!want && existing) {
    existing.remove()
  }
}

function fit() {
  const f = frame.value
  const d = frameDoc()
  if (f && d?.body) {
    const h = Math.max(d.body.scrollHeight, d.documentElement.scrollHeight)
    f.style.height = Math.min(h + 24, 20000) + 'px'
  }
}

function onLoad() {
  emailDark = measureEmailDark()
  measured = true
  applyOverride()
  fit()
  ready.value = true
}

// Theme/raw changes re-apply the override on the already-loaded document; a new
// email forces a fresh load + re-measure.
watch([dark, () => props.raw], () => {
  if (measured) applyOverride()
})
watch(
  () => props.html,
  () => {
    measured = false
    ready.value = false
  },
)
</script>

<template>
  <iframe
    ref="frame"
    :srcdoc="html"
    sandbox="allow-same-origin"
    referrerpolicy="no-referrer"
    loading="lazy"
    title="Full newsletter"
    :class="[
      'w-full bg-white rounded-lg ring-1 ring-black/5 transition-opacity duration-150',
      ready ? 'opacity-100' : 'opacity-0',
    ]"
    style="min-height: 240px"
    @load="onLoad"
  />
</template>
