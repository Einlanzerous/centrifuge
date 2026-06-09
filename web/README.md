# Centrifuge web

The reading UI — Vue 3 (`<script setup>`) + Vite + TypeScript + Tailwind. Recreates
the `design/` prototype against the CTFG-26 read API. See ticket **CTFG-27**.

## Develop

```sh
npm install
npm run dev          # http://localhost:5173, proxies /api + /feed.xml to the backend
```

The dev server proxies the API to `http://localhost:4003` by default (the
construct-server `centrifuge` container). Override with `CENTRIFUGE_API_PROXY`,
e.g. `CENTRIFUGE_API_PROXY=http://localhost:4004 npm run dev`.

`npm run build` type-checks (`vue-tsc`) and builds to `dist/`. `npm run typecheck`
runs the type check alone.

## Layout

- `src/api/` — typed client + DTOs mirroring `internal/httpapi` (keep in sync).
- `src/composables/` — `useCentrifuge` (shared store + optimistic mutations),
  `useTheme`, `useToast`.
- `src/components/` — `TopBar`, `HomeView` (Today), `ArchiveView`, `Card`,
  `ReaderModal`, `NewsletterFrame` (sandboxed-iframe email render), `Icon`, `Toast`.
- `src/lib/` — `art` (topic hue → gradient/dot), `format` (relative time).

## Production image

`Dockerfile` builds the SPA and serves it via nginx, proxying `/api` + `/feed.xml`
to `${CENTRIFUGE_UPSTREAM}` (default `centrifuge:8080`) so the app calls the
backend same-origin. Deploy wiring (release workflow + construct-server service)
is tracked in **CTFG-38**.
