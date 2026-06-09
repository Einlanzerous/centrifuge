<script setup lang="ts">
import Icon from './Icon.vue'
import { useToast } from '../composables/useToast'

const { toast } = useToast()
</script>

<template>
  <Transition name="pop">
    <div
      v-if="toast"
      class="fixed bottom-20 right-4 z-50 px-4 py-2.5 rounded-full bg-ink-800 dark:bg-ink-100 text-ink-50 dark:text-ink-800 text-sm font-medium shadow-2xl flex items-center gap-2"
    >
      <Icon name="check" :size="14" /> {{ toast }}
    </div>
  </Transition>
</template>
