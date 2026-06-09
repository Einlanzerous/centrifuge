import { ref } from 'vue'

const STORAGE_KEY = 'centrifuge:dark'

function initial(): boolean {
  return document.documentElement.classList.contains('dark')
}

const dark = ref(initial())

function apply(v: boolean) {
  dark.value = v
  document.documentElement.classList.toggle('dark', v)
  try {
    localStorage.setItem(STORAGE_KEY, String(v))
  } catch (_) {
    /* private mode */
  }
}

export function useTheme() {
  return {
    dark,
    setDark: apply,
    toggle: () => apply(!dark.value),
  }
}
