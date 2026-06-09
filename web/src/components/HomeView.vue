<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Item, Rating, TodayResponse } from '../api/types'
import { artGradient, dotColor, tagStyle } from '../lib/art'
import Card from './Card.vue'
import Icon from './Icon.vue'

const props = defineProps<{
  today: TodayResponse | null
  loading: boolean
}>()

const emit = defineEmits<{
  open: [item: Item]
  bookmark: [item: Item]
  rate: [item: Item, rating: Rating]
  'mark-ad': [item: Item]
  'spin-brief': []
  'go-archive': []
}>()

const items = computed(() => props.today?.items ?? [])
const topics = computed(() => props.today?.topics ?? [])
const showEmpty = computed(() => !!props.today && items.value.length === 0)

// Clicking a topic chip filters the masonry in place (toggle to clear).
const activeTopic = ref<string | null>(null)
watch(items, () => {
  if (activeTopic.value && !topics.value.some((t) => t.topic === activeTopic.value)) {
    activeTopic.value = null
  }
})
function toggleTopic(topic: string) {
  activeTopic.value = activeTopic.value === topic ? null : topic
}
const displayed = computed(() =>
  activeTopic.value ? items.value.filter((i) => i.primary_topic === activeTopic.value) : items.value,
)

const heroArt = artGradient(undefined)

const summaryLine = computed(() => {
  if (!items.value.length) return 'Centrifuge is quiet right now.'
  const top = topics.value.slice(0, 3).map((t) => t.topic.toLowerCase())
  const list =
    top.length === 1
      ? top[0]
      : top.length === 2
        ? `${top[0]} and ${top[1]}`
        : `${top[0]}, ${top[1]}, and ${top[2]}`
  return `Strongest signal in ${list}.`
})
</script>

<template>
  <section>
    <!-- Hero / Since you last looked -->
    <header class="mb-6 sm:mb-8">
      <h1
        class="font-serif text-[44px] sm:text-[56px] leading-[1.02] tracking-tight text-ink-800 dark:text-ink-50"
      >
        Since you last looked
        <em
          class="not-italic"
          :style="{
            backgroundImage:
              'linear-gradient(120deg,#e88a16 0%,#d63c8c 35%,#4f56e8 65%,#0fb39b 100%)',
            WebkitBackgroundClip: 'text',
            backgroundClip: 'text',
            color: 'transparent',
          }"
          >{{ today?.since_human ?? '…' }}</em
        >, here are the topics worth your time.
      </h1>

      <div class="mt-3 flex flex-wrap items-center gap-3 text-sm text-ink-500 dark:text-ink-300">
        <span class="font-mono text-[11px] uppercase tracking-widest">{{ items.length }} new</span>
        <span class="text-ink-300">·</span>
        <span>{{ summaryLine }}</span>
      </div>

      <!-- Topic chips — click to filter the feed below; click again to clear -->
      <div v-if="!showEmpty && topics.length" class="mt-4 flex flex-wrap gap-2">
        <button
          v-for="t in topics"
          :key="t.topic"
          class="tag transition hover:scale-[1.02]"
          :class="[
            activeTopic === t.topic ? 'ring-2 ring-ink-800 dark:ring-ink-100' : '',
            activeTopic && activeTopic !== t.topic ? 'opacity-50' : '',
          ]"
          :style="tagStyle(t.color)"
          :aria-pressed="activeTopic === t.topic"
          :title="activeTopic === t.topic ? `Clear ${t.topic} filter` : `Show only ${t.topic}`"
          @click="toggleTopic(t.topic)"
        >
          <span class="dot" :style="{ background: dotColor(t.color) }"></span>
          {{ t.topic }} · {{ t.count }}
        </button>
      </div>
    </header>

    <!-- Loading skeleton -->
    <div v-if="loading && !today" class="masonry">
      <div
        v-for="n in 6"
        :key="n"
        class="rounded-2xl bg-[var(--paper)] ring-1 ring-black/5 dark:ring-white/10 h-64 animate-pulse"
      ></div>
    </div>

    <!-- Empty state -->
    <div
      v-else-if="showEmpty"
      class="rounded-3xl ring-1 rule p-8 sm:p-10 bg-[var(--paper)] flex flex-col sm:flex-row gap-6 items-start sm:items-center"
    >
      <div class="art w-32 h-32 rounded-2xl flex-none" :style="{ '--art-bg': heroArt }"></div>
      <div class="flex-1">
        <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">
          No new mail
        </div>
        <h2 class="font-serif text-2xl leading-tight text-ink-800 dark:text-ink-50">
          Nothing new since you last looked.
        </h2>
        <p class="mt-2 text-ink-500 dark:text-ink-300">
          Want a brief spun up from older, unsurfaced stories instead? Centrifuge can pull together
          the most promising things you haven't read yet.
        </p>
        <div class="mt-4 flex gap-3">
          <button
            class="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-ink-800 dark:bg-ink-100 text-ink-50 dark:text-ink-800 text-sm font-medium hover:opacity-90"
            @click="emit('spin-brief')"
          >
            <Icon name="sparkle" :size="14" /> Spin today's brief
          </button>
          <button
            class="inline-flex items-center gap-2 px-4 py-2 rounded-full ring-1 rule text-ink-700 dark:text-ink-100 text-sm hover:bg-ink-100 dark:hover:bg-ink-700"
            @click="emit('go-archive')"
          >
            Browse archive <Icon name="arrow-right" :size="14" />
          </button>
        </div>
      </div>
    </div>

    <!-- Masonry -->
    <div v-else class="masonry">
      <Card
        v-for="item in displayed"
        :key="item.id"
        :item="item"
        @open="emit('open', $event)"
        @bookmark="emit('bookmark', $event)"
        @rate="(it, r) => emit('rate', it, r)"
        @mark-ad="emit('mark-ad', $event)"
      />
    </div>
  </section>
</template>
