# Centrifuge

Newsletter curator: a Go backend (`cmd/`, `internal/`) and a Vue 3 + Tailwind reading UI in `web/`.

## Browser target

Firefox is the primary browser. Floor: Firefox 160 (the Firefox release when this was set; the quarterly `frontend-audit` run raises it). Anything newer is progressive enhancement behind `@supports` or feature detection. The floor is encoded in `build.target` in `web/vite.config.ts`.

## Frontend

- `cd web && npm ci && npm run build` runs `vue-tsc` then `vite build`; there is no web test runner yet.
- Fonts are self-hosted through `@fontsource`; do not add third-party font or script origins.
- The reader is a native `<dialog>` opened with `showModal()`.
