import { ref } from 'vue'

const toast = ref<string | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined

export function useToast() {
  function show(message: string, ms = 2000) {
    toast.value = message
    clearTimeout(timer)
    timer = setTimeout(() => (toast.value = null), ms)
  }
  return { toast, show }
}
