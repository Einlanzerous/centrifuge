<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{ name: string; size?: number | string }>(), {
  size: 18,
})

// Sparse, functional iconography — line paths drawn on a 24×24 grid.
const PATHS: Record<string, string[]> = {
  bookmark: ['M6 3h12v18l-6-4-6 4z'],
  'thumb-up': ['M7 11v9H4v-9zM7 11l4-8a2 2 0 0 1 4 1l-1 5h5a2 2 0 0 1 2 2l-1.5 6a2 2 0 0 1-2 1.5H7'],
  'thumb-down': ['M17 13V4h3v9zM17 13l-4 8a2 2 0 0 1-4-1l1-5H5a2 2 0 0 1-2-2l1.5-6A2 2 0 0 1 6.5 5H17'],
  home: ['M3 11l9-7 9 7M5 10v10h14V10'],
  archive: ['M3 5h18M5 5v15h14V5M9 10h6'],
  search: ['M21 21l-4-4'],
  close: ['M6 6l12 12M18 6L6 18'],
  sparkle: ['M12 2v6M12 16v6M2 12h6M16 12h6M5 5l4 4M15 15l4 4M5 19l4-4M15 9l4-4'],
  check: ['M5 13l4 4 10-10'],
  sun: ['M12 3v2M12 19v2M3 12h2M19 12h2M5 5l1.5 1.5M17.5 17.5L19 19M5 19l1.5-1.5M17.5 6.5L19 5'],
  moon: ['M20 14A8 8 0 0 1 10 4a8 8 0 1 0 10 10z'],
  sliders: ['M4 6h10M18 6h2M4 12h2M10 12h10M4 18h14M20 18h0'],
  'arrow-right': ['M5 12h14M13 6l6 6-6 6'],
  ad: ['M4 4h16v16H4zM8 15v-4a2 2 0 0 1 4 0v4M8 13h4'],
}
const CIRCLES: Record<string, [number, number, number]> = {
  search: [11, 11, 7],
  sun: [12, 12, 4],
}

const paths = computed(() => PATHS[props.name] ?? [])
const circle = computed(() => CIRCLES[props.name])
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.6"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path v-for="p in paths" :key="p" :d="p" />
    <circle v-if="circle" :cx="circle[0]" :cy="circle[1]" :r="circle[2]" />
  </svg>
</template>
