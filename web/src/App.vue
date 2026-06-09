<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import type { Item } from './api/types'
import TopBar from './components/TopBar.vue'
import HomeView from './components/HomeView.vue'
import ArchiveView from './components/ArchiveView.vue'
import ReaderModal from './components/ReaderModal.vue'
import Toast from './components/Toast.vue'
import { useCentrifuge } from './composables/useCentrifuge'

const store = useCentrifuge()

const view = ref<'home' | 'archive'>('home')

onMounted(() => {
  void Promise.all([store.loadToday(), store.loadArchive()])

  // Advance last_viewed_at when the user leaves — closing or backgrounding the
  // tab — so the next visit's "since you last looked" measures from here. Doing
  // this on departure (rather than shortly after arrival) keeps today's feed
  // stable while it's being read and across refreshes.
  const markSeenOnLeave = () => {
    if (document.visibilityState === 'hidden' && (store.today.value?.items.length ?? 0) > 0) {
      void store.markSeen()
    }
  }
  document.addEventListener('visibilitychange', markSeenOnLeave)
  window.addEventListener('pagehide', markSeenOnLeave)
})

function navigate(v: 'home' | 'archive') {
  view.value = v
}

// Lightweight engagement counters from whatever is currently loaded.
const counts = computed(() => {
  const map = new Map<string, Item>()
  store.today.value?.items.forEach((it) => map.set(it.id, it))
  store.archive.value?.days.forEach((d) => d.items.forEach((it) => map.set(it.id, it)))
  let bookmark = 0
  let rated = 0
  map.forEach((it) => {
    if (it.bookmarked) bookmark++
    if (it.rating !== 'none') rated++
  })
  return { bookmark, rated }
})

const archiveCount = computed(() => store.archive.value?.total ?? 0)
</script>

<template>
  <div class="min-h-screen text-ink-800 dark:text-ink-100">
    <TopBar
      :view="view"
      :archive-count="archiveCount"
      :bookmark-count="counts.bookmark"
      :rated-count="counts.rated"
      @navigate="navigate"
    />

    <main class="max-w-[1400px] mx-auto px-5 sm:px-8 py-8 sm:py-12">
      <p
        v-if="store.error.value"
        class="mb-6 rounded-xl ring-1 rule bg-game-50 dark:bg-ink-800 px-4 py-3 text-sm text-game-700"
      >
        {{ store.error.value }}
      </p>

      <Transition name="fade" mode="out-in">
        <HomeView
          v-if="view === 'home'"
          key="home"
          :today="store.today.value"
          :loading="store.loadingToday.value"
          @open="store.open"
          @bookmark="store.toggleBookmark"
          @rate="store.rate"
          @mark-ad="store.markAd"
          @spin-brief="store.spinBrief"
          @go-archive="navigate('archive')"
        />

        <ArchiveView
          v-else
          key="archive"
          :archive="store.archive.value"
          :filter="store.filter"
          :loading="store.loadingArchive.value"
          @open="store.open"
          @bookmark="store.toggleBookmark"
          @rate="store.rate"
          @mark-ad="store.markAd"
          @set-filter="store.setFilter"
          @clear-filter="store.clearFilter"
        />
      </Transition>
    </main>

    <ReaderModal
      :item="store.reader.value"
      :loading="store.readerLoading.value"
      @close="store.closeReader"
      @bookmark="store.toggleBookmark"
      @rate="store.rate"
      @mark-ad="store.markAd"
    />

    <Toast />
  </div>
</template>
