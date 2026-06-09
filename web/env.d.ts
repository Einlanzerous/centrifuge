/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Base URL for the Centrifuge API. Empty = same-origin (dev proxy / nginx). */
  readonly VITE_API_BASE: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
