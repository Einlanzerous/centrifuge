import { reactive, ref } from 'vue'
import { api, ApiError } from '../api/client'
import type { ArchiveFilter, ArchiveResponse, Item, Rating, TodayResponse } from '../api/types'
import { useToast } from './useToast'

// A single shared store (module-scoped) so every view sees the same item state
// and engagement mutations stay consistent across Today, Archive, and Reader.

const today = ref<TodayResponse | null>(null)
const archive = ref<ArchiveResponse | null>(null)
const loadingToday = ref(false)
const loadingArchive = ref(false)
const error = ref<string | null>(null)

const reader = ref<Item | null>(null)
const readerLoading = ref(false)

// Archive query state, mapped 1:1 to the server-side filter (archive.go).
const filter = reactive<ArchiveFilter>({ topic: '', source: '', q: '', from: '' })

const { show } = useToast()

/** Run fn against every in-memory copy of the story with this id. */
function eachCopy(id: string, fn: (it: Item) => void) {
  today.value?.items.forEach((it) => it.id === id && fn(it))
  archive.value?.days.forEach((d) => d.items.forEach((it) => it.id === id && fn(it)))
  if (reader.value?.id === id) fn(reader.value)
}

/** Drop the story from the in-memory feeds (used when it becomes an ad). */
function removeEverywhere(id: string) {
  if (today.value) today.value.items = today.value.items.filter((it) => it.id !== id)
  if (archive.value) {
    archive.value.days = archive.value.days
      .map((d) => ({ ...d, items: d.items.filter((it) => it.id !== id) }))
      .filter((d) => d.items.length > 0)
  }
}

async function loadToday(brief = false) {
  loadingToday.value = true
  error.value = null
  try {
    today.value = await api.today(brief)
  } catch (e) {
    error.value = describe(e)
  } finally {
    loadingToday.value = false
  }
}

async function spinBrief() {
  await loadToday(true)
  if (today.value?.brief) show("Brief spun up from older stories.")
}

async function markSeen() {
  try {
    await api.markSeen()
  } catch (_) {
    /* best-effort; the next load will reflect it */
  }
}

function dateFloor(days: number): string {
  const d = new Date(Date.now() - days * 86400000)
  return d.toISOString().slice(0, 10) // YYYY-MM-DD
}

/** Translate a date-range preset key into the `from` param. */
export function rangeToFrom(range: string): string {
  switch (range) {
    case '24h':
      return dateFloor(1)
    case 'week':
      return dateFloor(7)
    case 'month':
      return dateFloor(30)
    default:
      return ''
  }
}

async function loadArchive() {
  loadingArchive.value = true
  error.value = null
  try {
    const f: ArchiveFilter = {}
    if (filter.topic) f.topic = filter.topic
    if (filter.source) f.source = filter.source
    if (filter.q) f.q = filter.q
    if (filter.from) f.from = filter.from
    archive.value = await api.archive(f)
  } catch (e) {
    error.value = describe(e)
  } finally {
    loadingArchive.value = false
  }
}

function setFilter(patch: Partial<ArchiveFilter & { range: string }>) {
  if ('range' in patch) {
    filter.from = rangeToFrom(patch.range as string)
    delete (patch as Record<string, unknown>).range
  }
  Object.assign(filter, patch)
  void loadArchive()
}

function clearFilter() {
  filter.topic = ''
  filter.source = ''
  filter.q = ''
  filter.from = ''
  void loadArchive()
}

async function open(item: Item) {
  reader.value = item // show immediately with what we have
  readerLoading.value = true
  try {
    const full = await api.item(item.id)
    reader.value = full
    eachCopy(item.id, (it) => (it.read = true))
  } catch (e) {
    show(describe(e))
  } finally {
    readerLoading.value = false
  }
}

function closeReader() {
  reader.value = null
}

async function toggleBookmark(item: Item) {
  const next = !item.bookmarked
  eachCopy(item.id, (it) => (it.bookmarked = next)) // optimistic
  try {
    const res = await api.bookmark(item.id)
    eachCopy(item.id, (it) => (it.bookmarked = res.bookmarked))
    show(res.bookmarked ? 'Bookmarked' : 'Removed bookmark')
  } catch (e) {
    eachCopy(item.id, (it) => (it.bookmarked = !next)) // revert
    show(describe(e))
  }
}

async function rate(item: Item, rating: Rating) {
  const prev = item.rating
  eachCopy(item.id, (it) => (it.rating = rating)) // optimistic
  try {
    const res = await api.rate(item.id, rating)
    eachCopy(item.id, (it) => (it.rating = res.rating))
    if (res.rating === 'up') show('Thanks — more like this.')
    else if (res.rating === 'down') show('Got it — less like this.')
  } catch (e) {
    eachCopy(item.id, (it) => (it.rating = prev)) // revert
    show(describe(e))
  }
}

async function markAd(item: Item) {
  try {
    await api.markAd(item.id)
    removeEverywhere(item.id)
    if (reader.value?.id === item.id) reader.value = null
    show('Marked as ad — hidden from your feeds.')
  } catch (e) {
    show(describe(e))
  }
}

function describe(e: unknown): string {
  if (e instanceof ApiError) {
    return e.status === 0 ? 'Cannot reach the server.' : e.message
  }
  return (e as Error).message || 'Something went wrong.'
}

export function useCentrifuge() {
  return {
    // state
    today,
    archive,
    loadingToday,
    loadingArchive,
    error,
    reader,
    readerLoading,
    filter,
    // actions
    loadToday,
    spinBrief,
    markSeen,
    loadArchive,
    setFilter,
    clearFilter,
    open,
    closeReader,
    toggleBookmark,
    rate,
    markAd,
  }
}
