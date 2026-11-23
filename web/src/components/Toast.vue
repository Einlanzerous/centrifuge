<script setup lang="ts">
import { ref, watch } from 'vue'
import Icon from './Icon.vue'
import { useToast } from '../composables/useToast'

const { toast } = useToast()

// The toast lives in the top layer (popover) so it stays visible above the
// modal reader <dialog>, which would otherwise cover a fixed z-index element.
const pop = ref<HTMLElement | null>(null)
watch(toast, (t) => {
  if (t && pop.value && !pop.value.matches(':popover-open')) pop.value.showPopover()
})
function onAfterLeave() {
  if (!toast.value) pop.value?.hidePopover()
}
</script>

<template>
  <!-- Always-present live region so assistive tech announces each message. -->
  <div class="sr-only" role="status" aria-live="polite">{{ toast }}</div>
  <div
    ref="pop"
    popover="manual"
    class="fixed inset-auto bottom-20 right-4 m-0 p-0 border-0 bg-transparent overflow-visible w-auto h-auto"
  >
    <Transition name="pop" @after-leave="onAfterLeave">
      <div
        v-if="toast"
        class="px-4 py-2.5 rounded-full bg-ink-800 dark:bg-ink-100 text-ink-50 dark:text-ink-800 text-sm font-medium shadow-2xl flex items-center gap-2"
        aria-hidden="true"
      >
        <Icon name="check" :size="14" /> {{ toast }}
      </div>
    </Transition>
  </div>
</template>
