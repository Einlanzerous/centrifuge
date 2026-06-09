<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { Item, Rating } from '../api/types'
import { artFallback, artGradient, dotColor } from '../lib/art'
import { formatDate, timeAgo } from '../lib/format'
import Icon from './Icon.vue'
import NewsletterFrame from './NewsletterFrame.vue'

const props = defineProps<{ item: Item | null; loading: boolean }>()

// Collapse the full-newsletter view (and reset the raw-colors toggle) whenever a
// different story opens.
const showFull = ref(false)
const rawNewsletter = ref(false)
watch(
  () => props.item?.id,
  () => {
    showFull.value = false
    rawNewsletter.value = false
  },
)

// The extracted article text keeps blank-line paragraph breaks; render each as
// its own <p>.
const contentParagraphs = computed(() =>
  (props.item?.content ?? '')
    .split(/\n{2,}/)
    .map((p) => p.trim())
    .filter(Boolean),
)

const emit = defineEmits<{
  close: []
  bookmark: [item: Item]
  rate: [item: Item, rating: Rating]
  'mark-ad': [item: Item]
}>()

const artBg = computed(() => artGradient(props.item?.topic_color))
const fallback = computed(() => artFallback(props.item?.topic_color))
const dot = computed(() => dotColor(props.item?.topic_color))

function nextRating(target: Rating): Rating {
  return props.item?.rating === target ? 'none' : target
}
</script>

<template>
  <Transition name="fade">
    <div
      v-if="item"
      class="fixed inset-0 z-40 bg-ink-900/40 dark:bg-black/70 backdrop-blur-sm flex items-stretch sm:items-center justify-center sm:p-8"
      @click.self="emit('close')"
    >
      <div
        class="bg-[var(--paper)] w-full sm:max-w-3xl sm:rounded-3xl shadow-2xl ring-1 ring-black/5 dark:ring-white/10 flex flex-col overflow-hidden max-h-[100vh] sm:max-h-[90vh]"
      >
        <!-- Header art: the story's own image when present, else topic gradient -->
        <div
          class="art h-40 sm:h-52 flex-none relative"
          :style="{ '--art-bg': artBg, '--art-fallback': fallback }"
        >
          <img
            v-if="item.image_url"
            :src="item.image_url"
            alt=""
            class="absolute inset-0 w-full h-full object-cover"
          />
          <div
            v-if="item.image_url"
            class="absolute inset-0 bg-gradient-to-t from-black/60 via-transparent to-black/20"
          ></div>
          <button
            class="absolute top-3 right-3 w-9 h-9 rounded-full bg-black/30 hover:bg-black/50 text-white flex items-center justify-center backdrop-blur"
            aria-label="Close"
            @click="emit('close')"
          >
            <Icon name="close" :size="18" />
          </button>
          <div class="absolute inset-x-0 bottom-0 p-4 sm:p-6 flex items-end justify-between gap-3">
            <div class="min-w-0">
              <span class="tag bg-white/20 backdrop-blur text-white">
                <span class="dot" :style="{ background: dot }"></span>{{ item.primary_topic || 'untagged' }}
              </span>
              <div class="mt-2 text-white/95 text-sm font-mono drop-shadow truncate">
                {{ item.source_name }} · {{ formatDate(item.received) }}
              </div>
            </div>
            <div class="flex items-center gap-2 flex-none">
              <button
                :class="[
                  'w-9 h-9 rounded-full flex items-center justify-center backdrop-blur transition',
                  item.rating === 'up' ? 'bg-white text-transit-700' : 'bg-white/20 text-white hover:bg-white/30',
                ]"
                aria-label="More like this"
                title="More like this"
                @click="emit('rate', item, nextRating('up'))"
              >
                <Icon name="thumb-up" :size="16" />
              </button>
              <button
                :class="[
                  'w-9 h-9 rounded-full flex items-center justify-center backdrop-blur transition',
                  item.rating === 'down' ? 'bg-white text-game-700' : 'bg-white/20 text-white hover:bg-white/30',
                ]"
                aria-label="Less like this"
                title="Less like this"
                @click="emit('rate', item, nextRating('down'))"
              >
                <Icon name="thumb-down" :size="16" />
              </button>
              <button
                :class="[
                  'w-9 h-9 rounded-full flex items-center justify-center backdrop-blur transition',
                  item.bookmarked ? 'bg-white text-ai-700' : 'bg-white/20 text-white hover:bg-white/30',
                ]"
                aria-label="Save"
                title="Save to bookmarks"
                @click="emit('bookmark', item)"
              >
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 24 24"
                  :fill="item.bookmarked ? 'currentColor' : 'none'"
                  stroke="currentColor"
                  stroke-width="1.6"
                  stroke-linejoin="round"
                >
                  <path d="M6 3h12v18l-6-4-6 4z" />
                </svg>
              </button>
            </div>
          </div>
        </div>

        <!-- Body -->
        <div
          class="reader flex-1 overflow-y-auto px-5 py-6 sm:px-10 sm:py-8 text-ink-700 dark:text-ink-100"
        >
          <h1 class="font-serif text-[2.25rem] leading-[1.05] mb-2 text-ink-800 dark:text-ink-50">
            {{ item.title }}
          </h1>
          <div v-if="loading" class="text-ink-400 font-mono text-xs uppercase tracking-widest mt-4">
            Loading…
          </div>

          <!-- Single essay: the newsletter body is the story; render isolated. -->
          <div v-else-if="!item.segmented && item.body" class="mt-4">
            <div class="flex justify-end mb-2">
              <button
                class="text-[11px] font-mono uppercase tracking-widest text-ink-400 hover:text-ink-700 dark:hover:text-ink-100"
                @click="rawNewsletter = !rawNewsletter"
              >
                {{ rawNewsletter ? 'Match theme' : 'Raw colors' }}
              </button>
            </div>
            <NewsletterFrame :html="item.body" :raw="rawNewsletter" />
          </div>

          <!-- Digest item: this story's own text, with the full newsletter on demand. -->
          <template v-else>
            <div class="prose-newsletter max-w-prose mt-4">
              <p
                v-if="item.section"
                class="!mt-0 font-mono text-[11px] uppercase tracking-widest text-ink-400"
              >
                {{ item.section }}
              </p>
              <!-- Verbatim article text sliced from the newsletter. -->
              <template v-if="item.content">
                <p
                  v-for="(para, i) in contentParagraphs"
                  :key="i"
                  class="text-[1.05rem] leading-relaxed"
                >
                  {{ para }}
                </p>
              </template>
              <!-- Fallback when extraction missed: the curated take + excerpt. -->
              <template v-else>
                <p v-if="item.summary" class="text-[1.05rem] leading-relaxed">{{ item.summary }}</p>
                <p
                  v-if="item.snippet && item.snippet !== item.summary"
                  class="text-ink-500 dark:text-ink-300"
                >
                  {{ item.snippet }}
                </p>
                <p
                  v-if="!item.summary && !item.snippet"
                  class="text-ink-400 font-mono text-xs uppercase tracking-widest"
                >
                  No text could be extracted for this story.
                </p>
              </template>
            </div>

            <div v-if="item.body" class="mt-6 border-t rule pt-4">
              <div class="flex items-center justify-between gap-3">
                <button
                  class="inline-flex items-center gap-1.5 text-sm font-medium text-ink-600 dark:text-ink-200 hover:text-ink-900 dark:hover:text-ink-50"
                  @click="showFull = !showFull"
                >
                  {{ showFull ? 'Hide full newsletter' : 'View full newsletter' }}
                  <span class="font-mono text-ink-400">{{ showFull ? '▲' : '▾' }}</span>
                </button>
                <button
                  v-if="showFull"
                  class="text-[11px] font-mono uppercase tracking-widest text-ink-400 hover:text-ink-700 dark:hover:text-ink-100"
                  @click="rawNewsletter = !rawNewsletter"
                >
                  {{ rawNewsletter ? 'Match theme' : 'Raw colors' }}
                </button>
              </div>
              <NewsletterFrame v-if="showFull" :html="item.body" :raw="rawNewsletter" class="mt-4" />
            </div>
          </template>
        </div>

        <!-- Footer -->
        <div
          class="px-5 sm:px-10 py-3 border-t rule flex items-center justify-between text-sm text-ink-500 dark:text-ink-300"
        >
          <div class="font-mono text-[11px] uppercase tracking-widest">{{ timeAgo(item.received) }}</div>
          <div class="flex gap-2">
            <button
              class="px-3 py-1.5 rounded-full text-game-700 hover:bg-game-50 dark:hover:bg-ink-700 text-sm"
              @click="emit('mark-ad', item)"
            >
              Mark as ad
            </button>
            <button
              class="px-3 py-1.5 rounded-full ring-1 rule text-ink-700 dark:text-ink-100 hover:bg-ink-100 dark:hover:bg-ink-700"
              @click="emit('close')"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>
