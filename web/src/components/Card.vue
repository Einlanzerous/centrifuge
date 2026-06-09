<script setup lang="ts">
import { computed } from 'vue'
import type { Item, Rating } from '../api/types'
import { artFallback, artGradient, artHeight, dotColor } from '../lib/art'
import { timeAgo } from '../lib/format'
import Icon from './Icon.vue'

const props = defineProps<{ item: Item }>()

const emit = defineEmits<{
  open: [item: Item]
  bookmark: [item: Item]
  rate: [item: Item, rating: Rating]
  'mark-ad': [item: Item]
}>()

const artBg = computed(() => artGradient(props.item.topic_color))
const fallback = computed(() => artFallback(props.item.topic_color))
const dot = computed(() => dotColor(props.item.topic_color))
const height = computed(() => artHeight(props.item.id))
const hasImage = computed(() => !!props.item.image_url)

function nextRating(target: Rating): Rating {
  return props.item.rating === target ? 'none' : target
}
</script>

<template>
  <article
    class="group relative rounded-2xl overflow-hidden bg-[var(--paper)] ring-1 ring-black/5 dark:ring-white/10 shadow-[0_1px_0_rgba(0,0,0,0.04),0_8px_24px_-12px_rgba(0,0,0,0.18)] hover:shadow-[0_1px_0_rgba(0,0,0,0.04),0_18px_36px_-12px_rgba(0,0,0,0.28)] transition-shadow"
  >
    <!-- Art header: the newsletter's own image when present, else a gradient
         generated from the topic hue. -->
    <div
      :class="['art', height]"
      :style="{ '--art-bg': artBg, '--art-fallback': fallback }"
      role="button"
      :aria-label="`Open ${item.title}`"
      @click="emit('open', item)"
    >
      <img
        v-if="hasImage"
        :src="item.image_url"
        alt=""
        loading="lazy"
        class="absolute inset-0 w-full h-full object-cover"
      />
      <div
        class="absolute inset-0 p-4 flex items-end"
        :class="hasImage ? 'bg-gradient-to-t from-black/50 via-transparent to-transparent' : ''"
      >
        <span class="tag bg-white/20 backdrop-blur text-white">
          <span class="dot" :style="{ background: dot }"></span>{{ item.primary_topic || 'untagged' }}
        </span>
      </div>
    </div>

    <!-- Body -->
    <div class="px-5 pb-5 pt-4">
      <div class="flex items-center gap-2 text-[11px] font-mono uppercase tracking-wider text-ink-400">
        <span class="truncate">{{ item.source_name }}</span>
        <span class="text-ink-300">·</span>
        <span>{{ timeAgo(item.received) }}</span>
      </div>

      <h3
        class="mt-2 font-serif text-[1.35rem] leading-[1.1] text-ink-800 dark:text-ink-50 cursor-pointer hover:underline underline-offset-2"
        @click="emit('open', item)"
      >
        {{ item.title }}
      </h3>

      <p
        v-if="item.summary"
        class="mt-2 text-[14px] leading-relaxed text-ink-600 dark:text-ink-200/90"
      >
        {{ item.summary }}
      </p>

      <!-- Action row -->
      <div class="mt-4 flex items-center justify-between">
        <div class="flex items-center gap-1">
          <button
            :class="[
              'p-1.5 rounded-full transition',
              item.rating === 'up'
                ? 'bg-transit-500/15 text-transit-700'
                : 'text-ink-400 hover:text-ink-700 hover:bg-ink-100 dark:hover:bg-ink-700',
            ]"
            :aria-pressed="item.rating === 'up'"
            aria-label="More like this"
            title="More like this — teaches Centrifuge to surface similar stories"
            @click.stop="emit('rate', item, nextRating('up'))"
          >
            <Icon name="thumb-up" :size="16" />
          </button>
          <button
            :class="[
              'p-1.5 rounded-full transition',
              item.rating === 'down'
                ? 'bg-game-500/15 text-game-700'
                : 'text-ink-400 hover:text-ink-700 hover:bg-ink-100 dark:hover:bg-ink-700',
            ]"
            :aria-pressed="item.rating === 'down'"
            aria-label="Less like this"
            title="Less like this — teaches Centrifuge to down-rank similar stories"
            @click.stop="emit('rate', item, nextRating('down'))"
          >
            <Icon name="thumb-down" :size="16" />
          </button>
        </div>

        <div class="flex items-center gap-1">
          <button
            class="p-1.5 rounded-full text-ink-400 hover:text-game-700 hover:bg-ink-100 dark:hover:bg-ink-700 transition opacity-0 group-hover:opacity-100 focus:opacity-100"
            aria-label="Mark as ad"
            title="Mark as ad — hides it and flags it as promotional"
            @click.stop="emit('mark-ad', item)"
          >
            <Icon name="ad" :size="16" />
          </button>
          <button
            :class="[
              'p-1.5 rounded-full transition',
              item.bookmarked
                ? 'text-ai-700 bg-ai-500/15'
                : 'text-ink-400 hover:text-ink-700 hover:bg-ink-100 dark:hover:bg-ink-700',
            ]"
            :aria-pressed="item.bookmarked"
            aria-label="Save"
            title="Save — keep this story in your bookmarks"
            @click.stop="emit('bookmark', item)"
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
  </article>
</template>
