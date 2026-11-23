# Review instructions

## Frontend

For changes under `web/`:

- Motion (transitions, animations, hover transforms) is gated on `prefers-reduced-motion` (`motion-safe:` / `motion-reduce:` or a media query).
- Modals use `<dialog>` and `showModal()`, not a fixed-position div with a scrim.
- Enter handlers on text inputs guard `isComposing` so IME composition is not submitted.
- No `outline: none` without a `:focus-visible` ring replacing it.
- New CSS and JS APIs are in Firefox 160 (the floor in `CLAUDE.md`) or behind `@supports` / feature detection with a working fallback.
- No `::-webkit-` rule without its standard property alongside.
- Clickable rows and cards are real `<button>` or `<a>` elements, not `<div @click>`.
