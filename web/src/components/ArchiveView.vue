<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ArchiveFilter, ArchiveResponse, Item, Rating } from '../api/types'
import { dotColor } from '../lib/art'
import { relTime } from '../lib/format'
import Icon from './Icon.vue'

const props = defineProps<{
  archive: ArchiveResponse | null
  filter: ArchiveFilter
  loading: boolean
}>()

const emit = defineEmits<{
  open: [item: Item]
  bookmark: [item: Item]
  rate: [item: Item, rating: Rating]
  'mark-ad': [item: Item]
  'set-filter': [patch: Partial<ArchiveFilter & { range: string }>]
  'clear-filter': []
}>()

const dateRanges = [
  { key: 'any', label: 'Any time' },
  { key: '24h', label: 'Last 24 hours' },
  { key: 'week', label: 'This week' },
  { key: 'month', label: 'This month' },
]

const topics = computed(() => props.archive?.topics ?? [])
const sources = computed(() => props.archive?.sources.filter((s) => s.story_count > 0) ?? [])
const days = computed(() => props.archive?.days ?? [])
const total = computed(() => props.archive?.total ?? 0)

// Derive the active date-range preset from the filter's `from` bound.
const activeRange = computed(() => {
  if (!props.filter.from) return 'any'
  const days = Math.round((Date.now() - new Date(props.filter.from).getTime()) / 86400000)
  if (days <= 1) return '24h'
  if (days <= 7) return 'week'
  return 'month'
})

const hasActiveFilter = computed(
  () => !!props.filter.q || !!props.filter.topic || !!props.filter.source || !!props.filter.from,
)

function toggleTopic(topic: string) {
  emit('set-filter', { topic: props.filter.topic === topic ? '' : topic })
}
function toggleSource(id: string) {
  emit('set-filter', { source: props.filter.source === id ? '' : id })
}

// Debounce the search box so we don't refetch on every keystroke.
const q = ref(props.filter.q ?? '')
watch(
  () => props.filter.q,
  (v) => {
    if ((v ?? '') !== q.value) q.value = v ?? ''
  },
)
let searchTimer: ReturnType<typeof setTimeout> | undefined
function onSearch(e: Event) {
  q.value = (e.target as HTMLInputElement).value
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => emit('set-filter', { q: q.value }), 250)
}
</script>

<template>
  <section class="grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-8">
    <!-- Sidebar filters -->
    <aside class="lg:sticky lg:top-24 self-start space-y-6">
      <div>
        <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">Topics</div>
        <ul class="space-y-1">
          <li>
            <button
              :class="[
                'w-full text-left px-2 py-1.5 rounded-md text-sm flex items-center justify-between',
                !filter.topic
                  ? 'bg-ink-100 dark:bg-ink-700 text-ink-800 dark:text-ink-50'
                  : 'text-ink-600 dark:text-ink-200 hover:bg-ink-100/60 dark:hover:bg-ink-700/60',
              ]"
              @click="emit('set-filter', { topic: '' })"
            >
              <span>All topics</span>
            </button>
          </li>
          <li v-for="t in topics" :key="t.topic">
            <button
              :class="[
                'w-full text-left px-2 py-1.5 rounded-md text-sm flex items-center gap-2 justify-between',
                filter.topic === t.topic
                  ? 'bg-ink-100 dark:bg-ink-700 text-ink-800 dark:text-ink-50'
                  : 'text-ink-600 dark:text-ink-200 hover:bg-ink-100/60 dark:hover:bg-ink-700/60',
              ]"
              @click="toggleTopic(t.topic)"
            >
              <span class="flex items-center gap-2 min-w-0">
                <span class="w-2 h-2 rounded-full flex-none" :style="{ background: dotColor(t.color) }"></span>
                <span class="truncate">{{ t.topic }}</span>
              </span>
              <span class="font-mono text-[11px] text-ink-400">{{ t.count }}</span>
            </button>
          </li>
        </ul>
      </div>

      <div>
        <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">Sources</div>
        <ul class="space-y-1 max-h-[280px] overflow-auto pr-1">
          <li v-for="s in sources" :key="s.id">
            <button
              :class="[
                'w-full flex items-center gap-2 px-2 py-1 rounded-md text-sm',
                filter.source === s.id
                  ? 'bg-ink-100 dark:bg-ink-700 text-ink-800 dark:text-ink-50'
                  : 'text-ink-700 dark:text-ink-100 hover:bg-ink-100/60 dark:hover:bg-ink-700/60',
              ]"
              @click="toggleSource(s.id)"
            >
              <span class="flex-1 truncate text-left">{{ s.name }}</span>
              <span class="font-mono text-[11px] text-ink-400">{{ s.story_count }}</span>
            </button>
          </li>
        </ul>
      </div>

      <div>
        <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">
          Date range
        </div>
        <div class="flex flex-col gap-1.5">
          <button
            v-for="r in dateRanges"
            :key="r.key"
            :class="[
              'text-left px-2 py-1.5 rounded-md text-sm',
              activeRange === r.key
                ? 'bg-ink-100 dark:bg-ink-700 text-ink-800 dark:text-ink-50'
                : 'text-ink-600 dark:text-ink-200 hover:bg-ink-100/60 dark:hover:bg-ink-700/60',
            ]"
            @click="emit('set-filter', { range: r.key })"
          >
            {{ r.label }}
          </button>
        </div>
      </div>

      <button
        v-if="hasActiveFilter"
        class="w-full text-left px-2 py-1.5 rounded-md text-sm text-game-700 hover:bg-game-50 dark:hover:bg-ink-700"
        @click="emit('clear-filter')"
      >
        Clear all filters
      </button>
    </aside>

    <!-- List -->
    <div>
      <header class="flex flex-wrap items-baseline gap-3 mb-5">
        <h1 class="font-serif text-4xl leading-tight text-ink-800 dark:text-ink-50">Archive</h1>
        <span class="font-mono text-[11px] uppercase tracking-widest text-ink-400"
          >{{ total }} items</span
        >
      </header>

      <!-- Search -->
      <div class="relative mb-4">
        <Icon
          name="search"
          :size="16"
          class="absolute left-3 top-1/2 -translate-y-1/2 text-ink-400"
        />
        <input
          type="text"
          :value="q"
          placeholder="Search titles, summaries, sources…"
          class="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[var(--paper)] ring-1 rule text-sm focus:ring-2 focus:ring-ink-300 focus:outline-none placeholder:text-ink-400"
          @input="onSearch"
        />
      </div>

      <div
        v-if="loading"
        class="rounded-2xl ring-1 rule p-8 text-center text-ink-500 dark:text-ink-300"
      >
        Loading…
      </div>
      <div
        v-else-if="days.length === 0"
        class="rounded-2xl ring-1 rule p-8 text-center text-ink-500 dark:text-ink-300"
      >
        No items match these filters.
      </div>
      <div v-else class="space-y-8">
        <div v-for="g in days" :key="g.date">
          <div class="flex items-center gap-3 mb-3">
            <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400">
              {{ g.label }}
            </div>
            <div class="flex-1 h-px bg-[var(--border)]"></div>
            <div class="font-mono text-[11px] text-ink-400">{{ g.items.length }}</div>
          </div>
          <ul
            class="divide-y rule rounded-2xl ring-1 rule bg-[var(--paper)] overflow-hidden"
          >
            <li
              v-for="it in g.items"
              :key="it.id"
              class="px-4 py-3.5 sm:px-5 sm:py-4 flex items-stretch gap-3 cursor-pointer hover:bg-ink-50 dark:hover:bg-ink-700/40 transition-colors"
              @click="emit('open', it)"
            >
              <div
                class="w-1 flex-none self-stretch rounded-full"
                :style="{ background: dotColor(it.topic_color) }"
              ></div>
              <div
                class="font-mono text-[11px] text-ink-400 w-7 flex-none self-center text-right leading-tight"
              >
                <div>{{ relTime(it.received).token }}</div>
                <div v-if="relTime(it.received).ago" class="text-ink-300">ago</div>
              </div>
              <div class="min-w-0 flex-1 self-center">
                <div
                  class="flex items-center gap-2 text-[11px] font-mono uppercase tracking-wider text-ink-400"
                >
                  <span class="truncate">{{ it.source_name }}</span>
                  <span class="text-ink-300">·</span>
                  <span class="truncate">{{ it.primary_topic }}</span>
                </div>
                <div
                  class="font-serif text-[1.15rem] leading-tight text-ink-800 dark:text-ink-50 truncate"
                >
                  {{ it.title }}
                </div>
                <div class="text-sm text-ink-500 dark:text-ink-300 line-clamp-1 mt-0.5">
                  {{ it.summary }}
                </div>
              </div>
              <div class="flex items-center gap-1 flex-none self-center">
                <button
                  v-if="it.bookmarked"
                  class="p-1.5 rounded-full text-ai-700 bg-ai-500/15"
                  aria-label="Bookmarked"
                  @click.stop="emit('bookmark', it)"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M6 3h12v18l-6-4-6 4z" />
                  </svg>
                </button>
                <span
                  v-if="it.rating === 'up'"
                  class="p-1.5 rounded-full text-transit-700 bg-transit-500/15"
                  ><Icon name="thumb-up" :size="14"
                /></span>
                <span
                  v-if="it.rating === 'down'"
                  class="p-1.5 rounded-full text-game-700 bg-game-500/15"
                  ><Icon name="thumb-down" :size="14"
                /></span>
              </div>
            </li>
          </ul>
        </div>
      </div>
    </div>
  </section>
</template>
