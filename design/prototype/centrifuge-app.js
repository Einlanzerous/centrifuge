/* Centrifuge — Newsletter Curator
 * Vue 3 Composition API (no build step; functionally equivalent to <script setup>)
 *
 * To port to a real Vue SFC project:
 *   1. Move <template> string into <template> block
 *   2. Move setup() body under <script setup>
 *   3. Drop the topics/data into composables (useTopics, useFeed, etc.)
 */

const { createApp, ref, reactive, computed, watch, onMounted, nextTick } = Vue;

/* ---------- Domain data ---------- */

const TOPICS = {
  ai:      { key:'ai',      label:'AI Engineering', short:'AI',         art:'art-ai' },
  transit: { key:'transit', label:'Transit & Urbanism', short:'Transit', art:'art-transit' },
  gov:     { key:'gov',     label:'Local Government', short:'Gov',      art:'art-gov' },
  nuc:     { key:'nuc',     label:'Nuclear Energy', short:'Nuclear',    art:'art-nuc' },
  game:    { key:'game',    label:'Grand Strategy', short:'Strategy',   art:'art-game' },
};

const SOURCES = [
  { id:'latent',   name:'Latent Space',         author:'swyx + alessio' },
  { id:'import',   name:'Import AI',            author:'Jack Clark' },
  { id:'citylab',  name:'Bloomberg CityLab',    author:'Editorial' },
  { id:'streets',  name:'Streetsblog Daily',    author:'M. Riordan' },
  { id:'council',  name:'Council Watch',        author:'Maya Iyer' },
  { id:'planning', name:'Planning Memo',        author:'D. Okafor' },
  { id:'atomic',   name:'Atomic Insights',      author:'R. Adams' },
  { id:'nucnews',  name:'Nuclear Newswire',     author:'Editorial' },
  { id:'paradox',  name:'Paradox Pulse',        author:'Jonas L.' },
  { id:'strategy', name:'Strategy Weekly',      author:'Anna V.' },
];

// Helper: hours ago → ISO
const now = new Date('2026-05-19T09:14:00');
const hoursAgo = (h) => new Date(now.getTime() - h * 3600 * 1000);

const FEED = [
  { id:'i1', topic:'ai', source:'latent',
    title:'Inference economics: the Q2 cost curve has bent again',
    summary:'Token throughput on hosted inference is down 41% QoQ as new batch-pipelining hits production. The note breaks down what that does to agent harness budgeting if you run multi-step workflows.',
    received: hoursAgo(3), read:false, important:true,
    body:`<h1>Inference economics: the Q2 cost curve has bent again</h1>
<p>The headline number — a 41% quarter-over-quarter drop in cost per million output tokens on the major hosted endpoints — is real, but the more interesting part is <em>why</em>. Three things are happening at once:</p>
<h2>1. Speculative decoding hit the mid-tier</h2>
<p>Until last quarter, speculative decoding was a frontier-only feature. It's now table stakes on every endpoint we benchmarked, and it disproportionately helps agent workflows where the model is doing structured outputs with predictable shape.</p>
<blockquote>If you've architected your agent harness assuming a 2024 cost basis, you are leaving a great deal of budget on the floor.</blockquote>
<h2>2. Batch APIs ate the long tail</h2>
<p>What used to be the "weekend backfill" workload is now a first-class deployment target. Three vendors shipped 24-hour batch APIs priced at roughly half of synchronous, and a fourth is in beta.</p>
<h2>3. Smaller models got disproportionately better</h2>
<p>The cheap-tier improvement on agentic coding evals (78% on SWE-Bench Verified, up from 51%) is the single largest delta we've seen since the GPT-3.5 era. For routing-style harnesses, this changes the math entirely.</p>` },

  { id:'i2', topic:'transit', source:'citylab',
    title:"Amtrak's $66B reauthorization quietly clears committee markup",
    summary:'Senate Commerce moved the surface transportation reauth out of markup with the long-distance network preserved. The interesting fight is over Gulf Coast restoration — back in scope, contingent on host railroad agreements by Q3.',
    received: hoursAgo(5), read:false,
    body:`<h1>Amtrak's $66B reauthorization quietly clears committee markup</h1>
<p>The Senate Commerce Committee approved the surface transportation reauthorization Wednesday with the bipartisan negotiating position largely intact. The bill preserves the national long-distance network through FY2031 and earmarks $66.1B in dedicated funding.</p>
<h2>What changed in markup</h2>
<p>The most interesting amendment was Sen. Wicker's substitute on Gulf Coast restoration. New Orleans–Mobile service is back in scope, but contingent on CSX and Norfolk Southern signing host railroad agreements by Q3 2026.</p>
<h2>What didn't make it</h2>
<p>The proposed $4B "station modernization corridor" grant program was struck from the bill. Expect it to reappear as a House add.</p>` },

  { id:'i3', topic:'gov', source:'council',
    title:'Burlington passes its zoning overhaul 9–3; ADU fees gutted',
    summary:'After 19 months of hearings, Burlington City Council adopted the Comprehensive Zoning Reform package on second reading. ADU permit fees drop from $4,400 to a flat $250. Three council members voted no on parking minimum elimination.',
    received: hoursAgo(8), read:false, important:true,
    body:`<h1>Burlington passes its zoning overhaul 9–3</h1>
<p>The Burlington City Council adopted the Comprehensive Zoning Reform package 9–3 on second reading Tuesday night, capping a 19-month process that began with the 2024 housing needs assessment.</p>
<h2>Key provisions that survived</h2>
<ul>
<li>ADU permit fees collapsed from $4,400 + impact charges to a flat $250.</li>
<li>Parking minimums eliminated citywide (this was the 9–3 split).</li>
<li>Missing-middle by-right up to 4-plex in all R-1 zones.</li>
<li>Form-based code adopted along three corridor overlays.</li>
</ul>` },

  { id:'i4', topic:'nuc', source:'atomic',
    title:'Westinghouse AP300 clears Phase 1 of NRC pre-application review',
    summary:'The 300-MWe SMR variant of the AP1000 just cleared its first regulatory milestone six months ahead of the published schedule. Three utility LOIs are pending, including one with TVA for the Clinch River site.',
    received: hoursAgo(10), read:false,
    body:`<h1>Westinghouse AP300 clears Phase 1 of NRC pre-application review</h1>
<p>The Nuclear Regulatory Commission completed Phase 1 of pre-application review for the AP300 small modular reactor design six months ahead of the published schedule, Westinghouse confirmed Thursday.</p>
<h2>What clearing Phase 1 actually means</h2>
<p>It is not a license. It is the regulator's confirmation that the design control document is sufficiently complete to begin technical review. But it does unlock the Part 52 combined license pathway, and it gives utility customers the regulatory certainty they need to sign firm orders.</p>` },

  { id:'i5', topic:'game', source:'paradox',
    title:'Victoria 3 patch 1.7 rebalances colonial economies — and the AI noticed',
    summary:"The 1.7 'Charter' patch overhauls how colonial extraction interacts with home-market industrialization. Early multiplayer reports say the AI is now playing a credible imperial Britain, which has not been true for two patches.",
    received: hoursAgo(14), read:false,
    body:`<h1>Victoria 3 patch 1.7 rebalances colonial economies</h1>
<p>The 1.7 "Charter" patch is the largest economic rework Victoria 3 has shipped since release. The headline change is that colonial extraction now feeds the home market through a separate accounting layer, which means the old "invade everything for the prestige" strategy stops being a free lunch.</p>
<h2>What the AI is doing now</h2>
<p>Multiplayer testers report that the AI is now playing a credible imperial Britain for the first time in two patches. This is probably the single most-requested fix in the community.</p>` },

  { id:'i6', topic:'ai', source:'import',
    title:'Why agent harnesses still leak tokens (and how to instrument for it)',
    summary:'A short, mostly diagnostic piece on the three most common ways agentic systems quietly burn 30–60% of their token budget on retries, redundant tool calls, and context that should have been compacted two turns ago.',
    received: hoursAgo(18), read:false },

  { id:'i7', topic:'transit', source:'streets',
    title:"California HSR Madera–Bakersfield extension hits its first contract milestone",
    summary:"The Authority awarded the CP-5 construction package on time and within the revised 2024 budget envelope. The political question — whether the IOS opens in 2031 or slips — now turns on rolling stock procurement, not civil works.",
    received: hoursAgo(22), read:false },

  { id:'i8', topic:'gov', source:'planning',
    title:'ADU permits hit a five-year high across 14 western cities',
    summary:'A combined dataset from 14 cities that liberalized accessory dwelling rules between 2019–2023 shows permit counts in 2025 at the highest level since reform. The piece argues the fee schedule matters more than the form-based code.',
    received: hoursAgo(26), read:false },

  { id:'i9', topic:'nuc', source:'nucnews',
    title:"TerraPower's Wyoming Natrium plant breaks ground on non-nuclear systems next month",
    summary:'The sodium test facility and the molten salt storage island can begin civil construction in June. The reactor building itself still waits on the NRC construction permit, expected late 2026.',
    received: hoursAgo(30), read:false },

  { id:'i10', topic:'game', source:'strategy',
    title:'EU4 "Domination" final patch notes: trade companies, again',
    summary:"After three years of post-launch development, the final EU4 patch returns to the trade company system one last time. Coring cost rebalanced, governing capacity recalculated. A sentimental read.",
    received: hoursAgo(35), read:false },

  { id:'i11', topic:'ai', source:'latent',
    title:'The retrieval stack has consolidated — here is what stuck',
    summary:'Two years after every RAG framework on the planet shipped v0.1, the dust has settled around three patterns. The piece is a calm, opinionated tour of what enterprise teams actually run in production.',
    received: hoursAgo(40), read:false },

  { id:'i12', topic:'transit', source:'citylab',
    title:'The post-mortem on the Vegas Loop nobody asked for, but here it is',
    summary:'Four years in, the Vegas Loop has moved fewer riders cumulatively than the monorail moves in a typical month. A careful, citation-heavy accounting of why the geometry never worked.',
    received: hoursAgo(48), read:false },

  /* Older items (for archive depth) */
  { id:'i13', topic:'gov', source:'council',
    title:'County stormwater fee survives ballot challenge',
    summary:'The 2024 stormwater utility fee survived its ballot initiative challenge by 11 points. Implementation continues on the existing schedule; the credit program opens applications next month.',
    received: hoursAgo(72), read:true },

  { id:'i14', topic:'nuc', source:'atomic',
    title:'Vogtle Unit 4 hits one-year operating anniversary',
    summary:'Capacity factor at Vogtle 4 came in at 91.4% over its first 12 months — slightly above the AP1000 fleet average and notably above what the early commissioning curve predicted.',
    received: hoursAgo(96), read:true },

  { id:'i15', topic:'ai', source:'import',
    title:'On the persistence of evaluation theater',
    summary:'A pointed, somewhat exasperated essay on why public leaderboard rankings keep diverging from internal eval scores for production teams, and what to actually do about it.',
    received: hoursAgo(120), read:true },
];

/* ---------- Format helpers ---------- */

function timeAgo(date, ref = now) {
  const s = Math.max(0, (ref - date) / 1000);
  if (s < 60) return 'just now';
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

function formatDate(d) {
  return d.toLocaleDateString('en-US', { month:'short', day:'numeric', year:'numeric' });
}

function formatLastVisit(d) {
  const diff = (now - d) / 36e5;
  if (diff < 1) return `${Math.round(diff*60)} minutes ago`;
  if (diff < 36) return `${Math.round(diff)} hours ago`;
  return `${Math.round(diff/24)} days ago`;
}

/* ---------- Iconography (sparse, only what's truly functional) ---------- */
const Icon = {
  template: `
    <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none"
         stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"
         aria-hidden="true"><path v-for="p in paths" :key="p" :d="p" /><circle
         v-if="circle" :cx="circle[0]" :cy="circle[1]" :r="circle[2]" /></svg>`,
  props: { name: String, size: { type: [Number, String], default: 18 } },
  computed: {
    paths() {
      const k = this.name;
      if (k === 'bookmark')   return ['M6 3h12v18l-6-4-6 4z'];
      if (k === 'bookmark-fill') return ['M6 3h12v18l-6-4-6 4z'];
      if (k === 'thumb-up')   return ['M7 11v9H4v-9zM7 11l4-8a2 2 0 0 1 4 1l-1 5h5a2 2 0 0 1 2 2l-1.5 6a2 2 0 0 1-2 1.5H7'];
      if (k === 'thumb-down') return ['M17 13V4h3v9zM17 13l-4 8a2 2 0 0 1-4-1l1-5H5a2 2 0 0 1-2-2l1.5-6A2 2 0 0 1 6.5 5H17'];
      if (k === 'home')       return ['M3 11l9-7 9 7M5 10v10h14V10'];
      if (k === 'archive')    return ['M3 5h18M5 5v15h14V5M9 10h6'];
      if (k === 'search')     return ['M21 21l-4-4'];
      if (k === 'filter')     return ['M3 5h18M6 12h12M10 19h4'];
      if (k === 'close')      return ['M6 6l12 12M18 6L6 18'];
      if (k === 'sparkle')    return ['M12 2v6M12 16v6M2 12h6M16 12h6M5 5l4 4M15 15l4 4M5 19l4-4M15 9l4-4'];
      if (k === 'check')      return ['M5 13l4 4 10-10'];
      if (k === 'sun')        return ['M12 3v2M12 19v2M3 12h2M19 12h2M5 5l1.5 1.5M17.5 17.5L19 19M5 19l1.5-1.5M17.5 6.5L19 5'];
      if (k === 'moon')       return ['M20 14A8 8 0 0 1 10 4a8 8 0 1 0 10 10z'];
      if (k === 'sliders')    return ['M4 6h10M18 6h2M4 12h2M10 12h10M4 18h14M20 18h0'];
      if (k === 'dot')        return [];
      if (k === 'inbox')      return ['M3 12l4-8h10l4 8v6H3zM3 12h5l1 3h6l1-3h5'];
      if (k === 'arrow-right')return ['M5 12h14M13 6l6 6-6 6'];
      return [];
    },
    circle() {
      if (this.name === 'search') return [11, 11, 7];
      if (this.name === 'sun')    return [12, 12, 4];
      if (this.name === 'sliders'){ /* circle handled via paths */ }
      if (this.name === 'dot')    return [12, 12, 4];
      return null;
    },
  },
};

/* ---------- TweaksPanel (lightweight, no starter component dependency) ---------- */

const TweaksPanel = {
  template: `
    <div v-if="open"
         class="fixed bottom-4 right-4 z-50 w-72 rounded-2xl bg-[var(--paper)] shadow-2xl ring-1 ring-black/5 dark:ring-white/10 backdrop-blur p-4">
      <div class="flex items-center justify-between mb-3">
        <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400">Tweaks</div>
        <button class="text-ink-400 hover:text-ink-700 dark:hover:text-ink-100" @click="close" aria-label="Close tweaks">
          <Icon name="close" size="16" /></button>
      </div>

      <div class="space-y-4 text-sm">
        <label class="block">
          <span class="text-ink-500 dark:text-ink-300 text-xs font-medium">Density</span>
          <div class="mt-1 inline-flex rounded-full bg-ink-100 dark:bg-ink-700 p-1">
            <button v-for="d in ['cozy','tight']" :key="d"
              @click="set('density', d)"
              :class="['px-3 py-1 rounded-full text-xs capitalize transition',
                       tweaks.density===d ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-100'
                                         : 'text-ink-500 dark:text-ink-300']">{{ d }}</button>
          </div>
        </label>

        <label class="block">
          <span class="text-ink-500 dark:text-ink-300 text-xs font-medium">Card art</span>
          <div class="mt-1 inline-flex rounded-full bg-ink-100 dark:bg-ink-700 p-1">
            <button v-for="d in ['vibrant','calm','none']" :key="d"
              @click="set('artStyle', d)"
              :class="['px-3 py-1 rounded-full text-xs capitalize transition',
                       tweaks.artStyle===d ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-100'
                                          : 'text-ink-500 dark:text-ink-300']">{{ d }}</button>
          </div>
        </label>

        <label class="block">
          <span class="text-ink-500 dark:text-ink-300 text-xs font-medium">Theme</span>
          <div class="mt-1 inline-flex rounded-full bg-ink-100 dark:bg-ink-700 p-1">
            <button @click="set('dark', false)"
              :class="['px-3 py-1 rounded-full text-xs flex items-center gap-1 transition',
                       !tweaks.dark ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-100'
                                    : 'text-ink-500 dark:text-ink-300']">
              <Icon name="sun" size="13" /> Light</button>
            <button @click="set('dark', true)"
              :class="['px-3 py-1 rounded-full text-xs flex items-center gap-1 transition',
                       tweaks.dark ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-100'
                                  : 'text-ink-500 dark:text-ink-300']">
              <Icon name="moon" size="13" /> Dark</button>
          </div>
        </label>

        <label class="block">
          <span class="text-ink-500 dark:text-ink-300 text-xs font-medium">Empty state</span>
          <div class="mt-1 inline-flex rounded-full bg-ink-100 dark:bg-ink-700 p-1">
            <button @click="set('emptyState', false)"
              :class="['px-3 py-1 rounded-full text-xs transition',
                       !tweaks.emptyState ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-100'
                                         : 'text-ink-500 dark:text-ink-300']">Has new</button>
            <button @click="set('emptyState', true)"
              :class="['px-3 py-1 rounded-full text-xs transition',
                       tweaks.emptyState ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-100'
                                        : 'text-ink-500 dark:text-ink-300']">No new</button>
          </div>
        </label>

        <label class="block">
          <span class="text-ink-500 dark:text-ink-300 text-xs font-medium">Hours since last visit</span>
          <input type="range" min="1" max="72" step="1"
                 :value="tweaks.hoursSince"
                 @input="set('hoursSince', +$event.target.value)"
                 class="w-full mt-1" />
          <div class="text-[11px] font-mono text-ink-400">{{ tweaks.hoursSince }}h</div>
        </label>
      </div>
    </div>

    <button v-else
      class="fixed bottom-4 right-4 z-40 w-11 h-11 rounded-full bg-ink-800 dark:bg-ink-100 text-ink-50 dark:text-ink-800 shadow-xl flex items-center justify-center hover:scale-105 transition"
      @click="$emit('open')" aria-label="Open tweaks">
      <Icon name="sliders" size="18" />
    </button>
  `,
  components: { Icon },
  props: { tweaks: Object, open: Boolean },
  emits: ['open','close','set'],
  methods: {
    set(k, v) { this.$emit('set', k, v); },
    close() { this.$emit('close'); },
  },
};

/* ---------- Card ---------- */

const Card = {
  template: `
    <article
      :class="['group relative rounded-2xl overflow-hidden bg-[var(--paper)] ring-1 ring-black/5 dark:ring-white/10',
               'shadow-[0_1px_0_rgba(0,0,0,0.04),0_8px_24px_-12px_rgba(0,0,0,0.18)]',
               'hover:shadow-[0_1px_0_rgba(0,0,0,0.04),0_18px_36px_-12px_rgba(0,0,0,0.28)] transition-shadow']">
      <!-- Art -->
      <div v-if="artStyle !== 'none'"
           :class="['art', topic.art, artStyle === 'calm' ? 'opacity-60' : '', heightClass]"
           @click="$emit('open', item)"
           role="button"
           aria-label="Open newsletter">
        <div class="absolute inset-0 p-4 flex items-end">
          <div class="flex items-center gap-2 text-white/95 drop-shadow-sm">
            <span class="tag bg-white/20 backdrop-blur text-white">
              <span class="dot" :style="{ background: dotColor }"></span>{{ topic.short }}
            </span>
          </div>
        </div>
      </div>
      <div v-else class="px-5 pt-5">
        <span class="tag" :style="tagStyle">
          <span class="dot" :style="{ background: dotColor }"></span>{{ topic.short }}
        </span>
      </div>

      <!-- Body -->
      <div :class="['px-5 pb-5', artStyle !== 'none' ? 'pt-4' : 'pt-3']">
        <div class="flex items-center gap-2 text-[11px] font-mono uppercase tracking-wider text-ink-400">
          <span class="truncate">{{ source.name }}</span>
          <span class="text-ink-300">·</span>
          <span>{{ timeAgo(item.received) }}</span>
        </div>

        <h3 class="mt-2 font-serif text-[1.35rem] leading-[1.1] text-ink-800 dark:text-ink-50 cursor-pointer hover:underline underline-offset-2"
            @click="$emit('open', item)">
          {{ item.title }}
        </h3>

        <p v-if="item.summary"
           class="mt-2 text-[14px] leading-relaxed text-ink-600 dark:text-ink-200/90">
          {{ item.summary }}
        </p>

        <!-- Action row -->
        <div class="mt-4 flex items-center justify-between">
          <div class="flex items-center gap-1">
            <button
              @click.stop="$emit('rate', item, item.rating === 'up' ? null : 'up')"
              :class="['p-1.5 rounded-full transition',
                       item.rating === 'up'
                        ? 'bg-transit-500/15 text-transit-700'
                        : 'text-ink-400 hover:text-ink-700 hover:bg-ink-100 dark:hover:bg-ink-700']"
              :aria-pressed="item.rating === 'up'"
              aria-label="Thumbs up">
              <Icon name="thumb-up" size="16" />
            </button>
            <button
              @click.stop="$emit('rate', item, item.rating === 'down' ? null : 'down')"
              :class="['p-1.5 rounded-full transition',
                       item.rating === 'down'
                        ? 'bg-game-500/15 text-game-700'
                        : 'text-ink-400 hover:text-ink-700 hover:bg-ink-100 dark:hover:bg-ink-700']"
              :aria-pressed="item.rating === 'down'"
              aria-label="Thumbs down">
              <Icon name="thumb-down" size="16" />
            </button>
          </div>

          <button
            @click.stop="$emit('bookmark', item)"
            :class="['p-1.5 rounded-full transition',
                     item.bookmarked
                      ? 'text-ai-700 bg-ai-500/15'
                      : 'text-ink-400 hover:text-ink-700 hover:bg-ink-100 dark:hover:bg-ink-700']"
            :aria-pressed="item.bookmarked"
            aria-label="Bookmark">
            <svg width="16" height="16" viewBox="0 0 24 24"
                 :fill="item.bookmarked ? 'currentColor' : 'none'"
                 stroke="currentColor" stroke-width="1.6" stroke-linejoin="round">
              <path d="M6 3h12v18l-6-4-6 4z"/>
            </svg>
          </button>
        </div>
      </div>
    </article>
  `,
  components: { Icon },
  props: { item: Object, artStyle: String, density: String },
  emits: ['open','bookmark','rate'],
  computed: {
    topic() { return TOPICS[this.item.topic]; },
    source() { return SOURCES.find(s => s.id === this.item.source); },
    heightClass() {
      // Vary height for masonry rhythm — derived from item id
      const h = parseInt(this.item.id.replace(/\D/g,''), 10) || 1;
      const heights = ['h-32','h-44','h-56','h-40','h-48'];
      return heights[h % heights.length];
    },
    dotColor() {
      return ({ ai:'#f6b961', transit:'#5be3c8', gov:'#8d92ff', nuc:'#d8f57f', game:'#ff8ec5' })[this.item.topic];
    },
    tagStyle() {
      const c = ({
        ai:      ['#fff5e6','#a5570b'],
        transit: ['#e6fbf7','#067a6a'],
        gov:     ['#eeefff','#2f33a6'],
        nuc:     ['#f3fbe0','#587f08'],
        game:    ['#fde9f3','#9c1d62'],
      })[this.item.topic];
      return { background: c[0], color: c[1] };
    },
  },
  methods: { timeAgo },
};

/* ---------- Home view ---------- */

const HomeView = {
  template: `
    <section>
      <!-- Hero / Since you last looked -->
      <header class="mb-6 sm:mb-8">
        <div class="flex items-baseline gap-3 flex-wrap">
          <h1 class="font-serif text-[44px] sm:text-[56px] leading-[1.02] tracking-tight text-ink-800 dark:text-ink-50">
            Since you last looked
            <em class="not-italic" :style="{
              backgroundImage:'linear-gradient(120deg,#e88a16 0%,#d63c8c 35%,#4f56e8 65%,#0fb39b 100%)',
              WebkitBackgroundClip:'text', backgroundClip:'text', color:'transparent'
            }">{{ lastVisit }}</em>,
            here are the topics worth your time.
          </h1>
        </div>

        <div class="mt-3 flex flex-wrap items-center gap-3 text-sm text-ink-500 dark:text-ink-300">
          <span class="font-mono text-[11px] uppercase tracking-widest">{{ newCount }} new</span>
          <span class="text-ink-300">·</span>
          <span>{{ summaryLine }}</span>
        </div>

        <!-- Topic chips -->
        <div v-if="!empty" class="mt-4 flex flex-wrap gap-2">
          <button
            v-for="t in topicSummary" :key="t.key"
            @click="$emit('filter-topic', t.key)"
            class="tag transition hover:scale-[1.02]"
            :style="t.style">
            <span class="dot" :style="{ background: t.dot }"></span>
            {{ t.label }} · {{ t.count }}
          </button>
        </div>
      </header>

      <!-- Empty state -->
      <div v-if="empty"
        class="rounded-3xl ring-1 rule p-8 sm:p-10 bg-[var(--paper)] flex flex-col sm:flex-row gap-6 items-start sm:items-center">
        <div class="art art-ai w-32 h-32 rounded-2xl flex-none"></div>
        <div class="flex-1">
          <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">No new mail</div>
          <h2 class="font-serif text-2xl leading-tight text-ink-800 dark:text-ink-50">
            Nothing landed in your inbox this morning.
          </h2>
          <p class="mt-2 text-ink-500 dark:text-ink-300">
            Want a brief spun up from older, unsurfaced stories instead?
            Centrifuge can pull together the ten most promising things you haven't read yet.
          </p>
          <div class="mt-4 flex gap-3">
            <button
              @click="$emit('rebuild')"
              class="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-ink-800 dark:bg-ink-100 text-ink-50 dark:text-ink-800 text-sm font-medium hover:opacity-90">
              <Icon name="sparkle" size="14" /> Spin today's brief
            </button>
            <button
              @click="$emit('go-archive')"
              class="inline-flex items-center gap-2 px-4 py-2 rounded-full ring-1 rule text-ink-700 dark:text-ink-100 text-sm hover:bg-ink-100 dark:hover:bg-ink-700">
              Browse archive <Icon name="arrow-right" size="14" />
            </button>
          </div>
        </div>
      </div>

      <!-- Masonry -->
      <div v-else class="masonry">
        <Card v-for="item in items" :key="item.id"
              :item="item" :art-style="artStyle" :density="density"
              @open="$emit('open', $event)"
              @bookmark="$emit('bookmark', $event)"
              @rate="(it, r) => $emit('rate', it, r)" />
      </div>
    </section>
  `,
  components: { Card, Icon },
  props: { items: Array, lastVisit: String, empty: Boolean, artStyle: String, density: String },
  emits: ['open','bookmark','rate','rebuild','go-archive','filter-topic'],
  computed: {
    newCount() { return this.items.length; },
    summaryLine() {
      if (!this.items.length) return 'Centrifuge is quiet this morning.';
      const top = [...this.topicSummary].sort((a,b)=>b.count-a.count).slice(0,3).map(t=>t.label.toLowerCase());
      const list = top.length === 1 ? top[0]
                : top.length === 2 ? `${top[0]} and ${top[1]}`
                : `${top[0]}, ${top[1]}, and ${top[2]}`;
      return `Strongest signal in ${list}.`;
    },
    topicSummary() {
      const groups = {};
      for (const it of this.items) {
        groups[it.topic] = (groups[it.topic] || 0) + 1;
      }
      const styles = {
        ai:      { bg:'#fff5e6', fg:'#a5570b', dot:'#e88a16' },
        transit: { bg:'#e6fbf7', fg:'#067a6a', dot:'#0fb39b' },
        gov:     { bg:'#eeefff', fg:'#2f33a6', dot:'#4f56e8' },
        nuc:     { bg:'#f3fbe0', fg:'#587f08', dot:'#7eb70d' },
        game:    { bg:'#fde9f3', fg:'#9c1d62', dot:'#d63c8c' },
      };
      return Object.entries(groups).map(([k, count]) => ({
        key: k, label: TOPICS[k].label, count,
        style: { background: styles[k].bg, color: styles[k].fg },
        dot: styles[k].dot,
      })).sort((a,b)=>b.count-a.count);
    },
  },
};

/* ---------- Archive view ---------- */

const ArchiveView = {
  template: `
    <section class="grid grid-cols-1 lg:grid-cols-[260px_1fr] gap-8">
      <!-- Sidebar filters -->
      <aside class="lg:sticky lg:top-24 self-start space-y-6">
        <div>
          <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">Topics</div>
          <ul class="space-y-1">
            <li>
              <button @click="$emit('set-filter', { topics: [] })"
                :class="['w-full text-left px-2 py-1.5 rounded-md text-sm flex items-center justify-between',
                         filter.topics.length === 0
                          ? 'bg-ink-100 dark:bg-ink-700 text-ink-800 dark:text-ink-50'
                          : 'text-ink-600 dark:text-ink-200 hover:bg-ink-100/60 dark:hover:bg-ink-700/60']">
                <span>All topics</span>
                <span class="font-mono text-[11px] text-ink-400">{{ totalCount }}</span>
              </button>
            </li>
            <li v-for="t in topics" :key="t.key">
              <button @click="toggleTopic(t.key)"
                :class="['w-full text-left px-2 py-1.5 rounded-md text-sm flex items-center gap-2 justify-between',
                         filter.topics.includes(t.key)
                          ? 'bg-ink-100 dark:bg-ink-700 text-ink-800 dark:text-ink-50'
                          : 'text-ink-600 dark:text-ink-200 hover:bg-ink-100/60 dark:hover:bg-ink-700/60']">
                <span class="flex items-center gap-2">
                  <span class="w-2 h-2 rounded-full" :style="{ background: t.dot }"></span>{{ t.label }}
                </span>
                <span class="font-mono text-[11px] text-ink-400">{{ t.count }}</span>
              </button>
            </li>
          </ul>
        </div>

        <div>
          <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">Sources</div>
          <ul class="space-y-1 max-h-[280px] overflow-auto pr-1">
            <li v-for="s in sourcesWithCount" :key="s.id">
              <label class="flex items-center gap-2 px-2 py-1 rounded-md text-sm hover:bg-ink-100/60 dark:hover:bg-ink-700/60 cursor-pointer">
                <input type="checkbox" class="rounded text-ink-700 focus:ring-ink-300"
                       :checked="filter.sources.includes(s.id)"
                       @change="toggleSource(s.id)" />
                <span class="flex-1 truncate text-ink-700 dark:text-ink-100">{{ s.name }}</span>
                <span class="font-mono text-[11px] text-ink-400">{{ s.count }}</span>
              </label>
            </li>
          </ul>
        </div>

        <div>
          <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400 mb-2">Date range</div>
          <div class="flex flex-col gap-1.5">
            <button v-for="r in dateRanges" :key="r.key"
              @click="$emit('set-filter', { range: r.key })"
              :class="['text-left px-2 py-1.5 rounded-md text-sm',
                       filter.range === r.key
                        ? 'bg-ink-100 dark:bg-ink-700 text-ink-800 dark:text-ink-50'
                        : 'text-ink-600 dark:text-ink-200 hover:bg-ink-100/60 dark:hover:bg-ink-700/60']">
              {{ r.label }}
            </button>
          </div>
        </div>

        <button v-if="hasActiveFilter"
          @click="$emit('clear-filter')"
          class="w-full text-left px-2 py-1.5 rounded-md text-sm text-game-700 hover:bg-game-50">
          Clear all filters
        </button>
      </aside>

      <!-- List -->
      <div>
        <header class="flex flex-wrap items-baseline gap-3 mb-5">
          <h1 class="font-serif text-4xl leading-tight text-ink-800 dark:text-ink-50">Archive</h1>
          <span class="font-mono text-[11px] uppercase tracking-widest text-ink-400">{{ filtered.length }} items</span>
        </header>

        <!-- Search -->
        <div class="relative mb-4">
          <Icon name="search" size="16" class="absolute left-3 top-1/2 -translate-y-1/2 text-ink-400" />
          <input type="text" :value="filter.q" @input="$emit('set-filter', { q: $event.target.value })"
            placeholder="Search titles, summaries, sources…"
            class="w-full pl-10 pr-4 py-2.5 rounded-xl bg-[var(--paper)] ring-1 rule text-sm focus:ring-2 focus:ring-ink-300 focus:outline-none placeholder:text-ink-400" />
        </div>

        <!-- Group by day -->
        <div v-if="filtered.length === 0" class="rounded-2xl ring-1 rule p-8 text-center text-ink-500">
          No items match these filters.
        </div>
        <div v-else class="space-y-8">
          <div v-for="g in groupedByDay" :key="g.key">
            <div class="flex items-center gap-3 mb-3">
              <div class="font-mono text-[11px] uppercase tracking-widest text-ink-400">{{ g.label }}</div>
              <div class="flex-1 h-px bg-[var(--border)]"></div>
              <div class="font-mono text-[11px] text-ink-400">{{ g.items.length }}</div>
            </div>
            <ul class="divide-y rule rounded-2xl ring-1 rule bg-[var(--paper)] overflow-hidden">
              <li v-for="it in g.items" :key="it.id"
                  @click="$emit('open', it)"
                  class="px-4 py-3.5 sm:px-5 sm:py-4 grid grid-cols-[auto_auto_1fr_auto] gap-4 items-center cursor-pointer hover:bg-ink-50 dark:hover:bg-ink-700/40 transition-colors">
                <div class="w-1.5 self-stretch rounded-full" :style="{ background: dotFor(it.topic) }"></div>
                <div class="font-mono text-[11px] text-ink-400 w-10 text-right">{{ timeAgo(it.received) }}</div>
                <div class="min-w-0">
                  <div class="flex items-center gap-2 text-[11px] font-mono uppercase tracking-wider text-ink-400">
                    <span class="truncate">{{ sourceName(it.source) }}</span>
                    <span class="text-ink-300">·</span>
                    <span class="truncate">{{ topicLabel(it.topic) }}</span>
                  </div>
                  <div class="font-serif text-[1.15rem] leading-tight text-ink-800 dark:text-ink-50 truncate">
                    {{ it.title }}
                  </div>
                  <div class="text-sm text-ink-500 dark:text-ink-300 line-clamp-1 mt-0.5">{{ it.summary }}</div>
                </div>
                <div class="flex items-center gap-1 self-start sm:self-center">
                  <button v-if="it.bookmarked"
                    class="p-1.5 rounded-full text-ai-700 bg-ai-500/15"
                    aria-label="Bookmarked" @click.stop="$emit('bookmark', it)">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M6 3h12v18l-6-4-6 4z"/></svg>
                  </button>
                  <span v-if="it.rating === 'up'" class="p-1.5 rounded-full text-transit-700 bg-transit-500/15"><Icon name="thumb-up" size="14"/></span>
                  <span v-if="it.rating === 'down'" class="p-1.5 rounded-full text-game-700 bg-game-500/15"><Icon name="thumb-down" size="14"/></span>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </section>
  `,
  components: { Icon },
  props: { items: Array, filter: Object },
  emits: ['open','bookmark','rate','set-filter','clear-filter'],
  data() {
    return {
      dateRanges: [
        { key:'any',    label:'Any time' },
        { key:'24h',    label:'Last 24 hours' },
        { key:'week',   label:'This week' },
        { key:'month',  label:'This month' },
      ],
    };
  },
  computed: {
    totalCount() { return this.items.length; },
    hasActiveFilter() {
      return this.filter.q || this.filter.topics.length || this.filter.sources.length || this.filter.range !== 'any';
    },
    filtered() {
      return this.items.filter(it => {
        if (this.filter.topics.length && !this.filter.topics.includes(it.topic)) return false;
        if (this.filter.sources.length && !this.filter.sources.includes(it.source)) return false;
        if (this.filter.q) {
          const q = this.filter.q.toLowerCase();
          const hay = `${it.title} ${it.summary||''} ${this.sourceName(it.source)}`.toLowerCase();
          if (!hay.includes(q)) return false;
        }
        const ranges = { '24h': 1, 'week': 7, 'month': 30 };
        if (ranges[this.filter.range]) {
          const cutoff = now.getTime() - ranges[this.filter.range] * 86400000;
          if (it.received.getTime() < cutoff) return false;
        }
        return true;
      });
    },
    groupedByDay() {
      const groups = new Map();
      for (const it of this.filtered) {
        const d = new Date(it.received.getFullYear(), it.received.getMonth(), it.received.getDate());
        const key = d.toISOString();
        if (!groups.has(key)) groups.set(key, []);
        groups.get(key).push(it);
      }
      const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
      const out = [];
      for (const [key, arr] of [...groups.entries()].sort((a,b)=> new Date(b[0]) - new Date(a[0]))) {
        const d = new Date(key);
        let label;
        if (d.getTime() === today) label = 'Today';
        else if (d.getTime() === today - 86400000) label = 'Yesterday';
        else label = d.toLocaleDateString('en-US', { weekday:'long', month:'long', day:'numeric' });
        out.push({ key, label, items: arr.sort((a,b)=>b.received-a.received) });
      }
      return out;
    },
    topics() {
      return Object.values(TOPICS).map(t => {
        const dotMap = { ai:'#e88a16', transit:'#0fb39b', gov:'#4f56e8', nuc:'#7eb70d', game:'#d63c8c' };
        return {
          ...t,
          dot: dotMap[t.key],
          count: this.items.filter(i => i.topic === t.key).length,
        };
      });
    },
    sourcesWithCount() {
      return SOURCES.map(s => ({ ...s, count: this.items.filter(i => i.source === s.id).length }))
        .filter(s => s.count > 0)
        .sort((a,b) => b.count - a.count);
    },
  },
  methods: {
    timeAgo,
    sourceName(id) { return (SOURCES.find(s=>s.id===id) || {}).name || id; },
    topicLabel(k) { return TOPICS[k].label; },
    dotFor(k) { return ({ ai:'#e88a16', transit:'#0fb39b', gov:'#4f56e8', nuc:'#7eb70d', game:'#d63c8c' })[k]; },
    toggleTopic(k) {
      const t = this.filter.topics.includes(k)
        ? this.filter.topics.filter(x => x !== k)
        : [...this.filter.topics, k];
      this.$emit('set-filter', { topics: t });
    },
    toggleSource(id) {
      const s = this.filter.sources.includes(id)
        ? this.filter.sources.filter(x => x !== id)
        : [...this.filter.sources, id];
      this.$emit('set-filter', { sources: s });
    },
  },
};

/* ---------- Reader modal ---------- */

const ReaderModal = {
  template: `
    <transition name="fade">
      <div v-if="item"
           class="fixed inset-0 z-40 bg-ink-900/40 dark:bg-black/70 backdrop-blur-sm flex items-stretch sm:items-center justify-center sm:p-8"
           @click.self="$emit('close')">
        <div class="bg-[var(--paper)] w-full sm:max-w-3xl sm:rounded-3xl shadow-2xl ring-1 ring-black/5 dark:ring-white/10 flex flex-col overflow-hidden max-h-[100vh] sm:max-h-[90vh]">
          <!-- Header art -->
          <div :class="['art', topic.art, 'h-40 sm:h-52 flex-none relative']">
            <button class="absolute top-3 right-3 w-9 h-9 rounded-full bg-black/30 hover:bg-black/50 text-white flex items-center justify-center backdrop-blur"
              @click="$emit('close')" aria-label="Close">
              <Icon name="close" size="18" />
            </button>
            <div class="absolute inset-x-0 bottom-0 p-4 sm:p-6 flex items-end justify-between">
              <div>
                <span class="tag bg-white/20 backdrop-blur text-white">
                  <span class="dot" style="background:#fff"></span>{{ topic.label }}
                </span>
                <div class="mt-2 text-white/95 text-sm font-mono drop-shadow">
                  {{ source.name }} · {{ source.author }} · {{ formatDate(item.received) }}
                </div>
              </div>
              <div class="flex items-center gap-2">
                <button @click="$emit('rate', item, item.rating === 'up' ? null : 'up')"
                  :class="['w-9 h-9 rounded-full flex items-center justify-center backdrop-blur transition',
                           item.rating === 'up' ? 'bg-white text-transit-700' : 'bg-white/20 text-white hover:bg-white/30']"
                  aria-label="Thumbs up"><Icon name="thumb-up" size="16" /></button>
                <button @click="$emit('rate', item, item.rating === 'down' ? null : 'down')"
                  :class="['w-9 h-9 rounded-full flex items-center justify-center backdrop-blur transition',
                           item.rating === 'down' ? 'bg-white text-game-700' : 'bg-white/20 text-white hover:bg-white/30']"
                  aria-label="Thumbs down"><Icon name="thumb-down" size="16" /></button>
                <button @click="$emit('bookmark', item)"
                  :class="['w-9 h-9 rounded-full flex items-center justify-center backdrop-blur transition',
                           item.bookmarked ? 'bg-white text-ai-700' : 'bg-white/20 text-white hover:bg-white/30']"
                  aria-label="Bookmark">
                  <svg width="16" height="16" viewBox="0 0 24 24"
                       :fill="item.bookmarked ? 'currentColor' : 'none'"
                       stroke="currentColor" stroke-width="1.6" stroke-linejoin="round">
                    <path d="M6 3h12v18l-6-4-6 4z"/></svg>
                </button>
              </div>
            </div>
          </div>

          <!-- Body -->
          <div class="reader flex-1 overflow-y-auto px-5 py-6 sm:px-10 sm:py-8 text-ink-700 dark:text-ink-100">
            <div v-if="item.body" class="prose-newsletter max-w-prose" v-html="item.body"></div>
            <div v-else class="prose-newsletter max-w-prose">
              <h1>{{ item.title }}</h1>
              <p class="text-ink-500">{{ item.summary }}</p>
              <p class="text-ink-400 font-mono text-xs uppercase tracking-widest mt-8">
                The full body of this newsletter would be rendered here from the original HTML.
              </p>
            </div>
          </div>

          <!-- Footer -->
          <div class="px-5 sm:px-10 py-3 border-t rule flex items-center justify-between text-sm text-ink-500 dark:text-ink-300">
            <div class="font-mono text-[11px] uppercase tracking-widest">{{ timeAgo(item.received) }}</div>
            <div class="flex gap-2">
              <button @click="$emit('close')"
                class="px-3 py-1.5 rounded-full ring-1 rule text-ink-700 dark:text-ink-100 hover:bg-ink-100 dark:hover:bg-ink-700">
                Close
              </button>
            </div>
          </div>
        </div>
      </div>
    </transition>
  `,
  components: { Icon },
  props: { item: Object },
  emits: ['close','bookmark','rate'],
  computed: {
    topic() { return this.item ? TOPICS[this.item.topic] : {}; },
    source() { return this.item ? SOURCES.find(s => s.id === this.item.source) : {}; },
  },
  methods: { timeAgo, formatDate },
};

/* ---------- Root App ---------- */

const App = {
  template: `
    <div :class="[tweaks.dark ? 'dark' : '', tweaks.density === 'tight' ? 'density-tight' : '']">
      <div class="min-h-screen text-ink-800 dark:text-ink-100">

        <!-- Top bar -->
        <header class="sticky top-0 z-30 backdrop-blur bg-[color:var(--bg)]/80 border-b rule">
          <div class="max-w-[1400px] mx-auto px-5 sm:px-8 py-3.5 flex items-center gap-4">
            <div class="flex items-center gap-2">
              <!-- Mark -->
              <div class="relative w-8 h-8 rounded-lg overflow-hidden">
                <div class="absolute inset-0"
                     style="background: conic-gradient(from 200deg at 50% 50%, #e88a16, #d63c8c, #4f56e8, #0fb39b, #e88a16);"></div>
                <div class="absolute inset-[3px] rounded-md bg-[var(--paper)]"></div>
                <div class="absolute inset-0 flex items-center justify-center text-[15px] font-serif text-ink-800 dark:text-ink-50">C</div>
              </div>
              <div class="font-serif text-[22px] leading-none text-ink-800 dark:text-ink-50">Centrifuge</div>
            </div>

            <!-- Tabs -->
            <nav class="ml-2 sm:ml-6 inline-flex rounded-full bg-ink-100 dark:bg-ink-700 p-1">
              <button @click="view = 'home'"
                :class="['px-3 sm:px-4 py-1.5 rounded-full text-sm font-medium flex items-center gap-2 transition',
                         view === 'home' ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-50' : 'text-ink-500 dark:text-ink-200']">
                <Icon name="home" size="14" /> Today
              </button>
              <button @click="view = 'archive'"
                :class="['px-3 sm:px-4 py-1.5 rounded-full text-sm font-medium flex items-center gap-2 transition',
                         view === 'archive' ? 'bg-[var(--paper)] shadow text-ink-800 dark:text-ink-50' : 'text-ink-500 dark:text-ink-200']">
                <Icon name="archive" size="14" /> Archive
                <span class="font-mono text-[10px] text-ink-400">{{ items.length }}</span>
              </button>
            </nav>

            <div class="flex-1"></div>

            <div class="hidden sm:flex items-center gap-3 text-sm text-ink-500 dark:text-ink-300">
              <span class="font-mono text-[11px] uppercase tracking-widest">{{ bookmarkCount }} bookmarked</span>
              <span class="text-ink-300">·</span>
              <span class="font-mono text-[11px] uppercase tracking-widest">{{ ratedCount }} rated</span>
            </div>
          </div>
        </header>

        <main class="max-w-[1400px] mx-auto px-5 sm:px-8 py-8 sm:py-12">
          <transition name="fade" mode="out-in">
            <HomeView v-if="view === 'home'"
              key="home"
              :items="visibleHomeItems"
              :last-visit="lastVisitLabel"
              :empty="tweaks.emptyState"
              :art-style="tweaks.artStyle"
              :density="tweaks.density"
              @open="openItem"
              @bookmark="toggleBookmark"
              @rate="rateItem"
              @rebuild="rebuildGlance"
              @go-archive="view = 'archive'"
              @filter-topic="filterByTopic" />

            <ArchiveView v-else
              key="archive"
              :items="items"
              :filter="filter"
              @open="openItem"
              @bookmark="toggleBookmark"
              @rate="rateItem"
              @set-filter="setFilter"
              @clear-filter="clearFilter" />
          </transition>
        </main>

        <!-- Reader modal -->
        <ReaderModal :item="readingItem"
          @close="readingItem = null"
          @bookmark="toggleBookmark"
          @rate="rateItem" />

        <!-- Toast for empty-state rebuild -->
        <transition name="pop">
          <div v-if="toast"
            class="fixed bottom-20 right-4 z-50 px-4 py-2.5 rounded-full bg-ink-800 dark:bg-ink-100 text-ink-50 dark:text-ink-800 text-sm font-medium shadow-2xl flex items-center gap-2">
            <Icon name="check" size="14" /> {{ toast }}
          </div>
        </transition>

        <!-- Tweaks panel -->
        <TweaksPanel :tweaks="tweaks" :open="tweaksOpen"
          @open="tweaksOpen = true" @close="tweaksOpen = false" @set="setTweak" />
      </div>
    </div>
  `,
  components: { HomeView, ArchiveView, ReaderModal, TweaksPanel, Icon },
  setup() {
    // Items are reactive; we mutate bookmark / rating / read state in place.
    const items = reactive(FEED.map(i => ({
      ...i,
      bookmarked: false,
      rating: null,
    })));

    const view = ref('home');
    const readingItem = ref(null);
    const tweaksOpen = ref(false);
    const toast = ref(null);

    const tweaks = reactive(/*EDITMODE-BEGIN*/{
      "density": "cozy",
      "artStyle": "vibrant",
      "dark": false,
      "emptyState": false,
      "hoursSince": 6
    }/*EDITMODE-END*/);

    const filter = reactive({
      q: '',
      topics: [],
      sources: [],
      range: 'any',
    });

    /* Derived */
    const lastVisitLabel = computed(() => {
      const h = tweaks.hoursSince;
      if (h < 1) return `${Math.round(h*60)} minutes ago`;
      if (h < 36) return `${h} hours ago`;
      return `${Math.round(h/24)} days ago`;
    });

    const visibleHomeItems = computed(() => {
      const cutoff = now.getTime() - tweaks.hoursSince * 3600 * 1000;
      return items.filter(i => i.received.getTime() >= cutoff)
                  .sort((a,b) => b.received - a.received);
    });

    const bookmarkCount = computed(() => items.filter(i => i.bookmarked).length);
    const ratedCount = computed(() => items.filter(i => i.rating).length);

    /* Mutations */
    function openItem(it) {
      readingItem.value = it;
      it.read = true;
    }
    function toggleBookmark(it) {
      it.bookmarked = !it.bookmarked;
      showToast(it.bookmarked ? 'Bookmarked' : 'Removed bookmark');
    }
    function rateItem(it, r) {
      it.rating = r;
      if (r === 'up') showToast('Thanks — more like this.');
      if (r === 'down') showToast('Got it — less like this.');
    }
    function setFilter(patch) {
      Object.assign(filter, patch);
    }
    function clearFilter() {
      filter.q = '';
      filter.topics = [];
      filter.sources = [];
      filter.range = 'any';
    }
    function filterByTopic(k) {
      view.value = 'archive';
      filter.topics = [k];
    }
    function rebuildGlance() {
      tweaks.emptyState = false;
      showToast('Brief spun up from older stories.');
    }
    function setTweak(k, v) {
      tweaks[k] = v;
      window.parent && window.parent.postMessage(
        { type: '__edit_mode_set_keys', edits: { [k]: v } }, '*');
    }

    let toastTimer;
    function showToast(msg) {
      toast.value = msg;
      clearTimeout(toastTimer);
      toastTimer = setTimeout(() => (toast.value = null), 2000);
    }

    /* Edit-mode bridge (Tweaks toolbar toggle) */
    onMounted(() => {
      window.addEventListener('message', (e) => {
        if (!e.data || typeof e.data !== 'object') return;
        if (e.data.type === '__activate_edit_mode') tweaksOpen.value = true;
        if (e.data.type === '__deactivate_edit_mode') tweaksOpen.value = false;
      });
      window.parent && window.parent.postMessage({ type: '__edit_mode_available' }, '*');
    });

    watch(() => tweaksOpen.value, (v) => {
      if (!v) window.parent && window.parent.postMessage({ type: '__edit_mode_dismissed' }, '*');
    });

    return {
      items, view, readingItem, tweaksOpen, tweaks, filter, toast,
      lastVisitLabel, visibleHomeItems, bookmarkCount, ratedCount,
      openItem, toggleBookmark, rateItem, setFilter, clearFilter,
      filterByTopic, rebuildGlance, setTweak,
    };
  },
};

createApp(App).mount('#app');
