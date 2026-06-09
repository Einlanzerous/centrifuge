<script setup lang="ts">
import Icon from './Icon.vue'
import { useTheme } from '../composables/useTheme'

defineProps<{
  view: 'home' | 'archive'
  archiveCount: number
  bookmarkCount: number
  ratedCount: number
}>()

const emit = defineEmits<{ navigate: [view: 'home' | 'archive'] }>()

const { dark, toggle } = useTheme()
</script>

<template>
  <header class="sticky top-0 z-30 backdrop-blur bg-[color:var(--bg)]/80 border-b rule">
    <div class="max-w-[1400px] mx-auto px-5 sm:px-8 py-3.5 flex items-center gap-4">
      <div class="flex items-center gap-2">
        <!-- Mark -->
        <div class="relative w-8 h-8 rounded-lg overflow-hidden">
          <div
            class="absolute inset-0"
            style="
              background: conic-gradient(
                from 200deg at 50% 50%,
                #e88a16,
                #d63c8c,
                #4f56e8,
                #0fb39b,
                #e88a16
              );
            "
          ></div>
          <div class="absolute inset-[3px] rounded-md bg-[var(--paper)]"></div>
          <div
            class="absolute inset-0 flex items-center justify-center text-[15px] font-serif text-ink-800 dark:text-ink-50"
          >
            C
          </div>
        </div>
        <div class="font-serif text-[22px] leading-none text-ink-800 dark:text-ink-50">
          Centrifuge
        </div>
      </div>

      <!-- Tabs -->
      <nav class="ml-2 sm:ml-6 inline-flex rounded-full bg-ink-100 dark:bg-ink-700 p-1">
        <button
          :class="[
            'px-3 sm:px-4 py-1.5 rounded-full text-sm font-medium flex items-center gap-2 transition',
            view === 'home'
              ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-50'
              : 'text-ink-500 dark:text-ink-200',
          ]"
          @click="emit('navigate', 'home')"
        >
          <Icon name="home" :size="14" /> Today
        </button>
        <button
          :class="[
            'px-3 sm:px-4 py-1.5 rounded-full text-sm font-medium flex items-center gap-2 transition',
            view === 'archive'
              ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-50'
              : 'text-ink-500 dark:text-ink-200',
          ]"
          @click="emit('navigate', 'archive')"
        >
          <Icon name="archive" :size="14" /> Archive
          <span class="font-mono text-[10px] text-ink-400">{{ archiveCount }}</span>
        </button>
      </nav>

      <div class="flex-1"></div>

      <div class="hidden sm:flex items-center gap-3 text-sm text-ink-500 dark:text-ink-300">
        <span class="font-mono text-[11px] uppercase tracking-widest">{{ bookmarkCount }} bookmarked</span>
        <span class="text-ink-300">·</span>
        <span class="font-mono text-[11px] uppercase tracking-widest">{{ ratedCount }} rated</span>
      </div>

      <button
        class="ml-1 w-9 h-9 rounded-full flex items-center justify-center text-ink-500 dark:text-ink-300 hover:bg-ink-100 dark:hover:bg-ink-700 transition"
        :aria-label="dark ? 'Switch to light mode' : 'Switch to dark mode'"
        :title="dark ? 'Light mode' : 'Dark mode'"
        @click="toggle"
      >
        <Icon :name="dark ? 'sun' : 'moon'" :size="16" />
      </button>
    </div>
  </header>
</template>
