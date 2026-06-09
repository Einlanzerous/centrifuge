# Changelog

## [1.2.0](https://github.com/Einlanzerous/centrifuge/compare/v1.1.1...v1.2.0) (2026-06-09)


### Features

* build & push the frontend image on release (CTFG-38) ([f8a62ea](https://github.com/Einlanzerous/centrifuge/commit/f8a62ea20ddafe32c7873ede223480449468494f))
* build & push the frontend image on release (CTFG-38) ([17f2a1a](https://github.com/Einlanzerous/centrifuge/commit/17f2a1ab568509f572d9b3427ec21f0815812455))
* reader read-API extensions — hero image, per-story text, segmented body (CTFG-27) ([284b15c](https://github.com/Einlanzerous/centrifuge/commit/284b15cf54bf0927379ec731f15cfdf187c11659))
* Vue 3 + Tailwind reading UI (CTFG-27) ([66f1f4f](https://github.com/Einlanzerous/centrifuge/commit/66f1f4f919270fd23ec93ac4fc9da514c0f49a55))
* Vue 3 + Tailwind reading UI (CTFG-27) ([23f236b](https://github.com/Einlanzerous/centrifuge/commit/23f236b5ee50828495cbe2612dcaa6551d31c71c))

## [1.1.1](https://github.com/Einlanzerous/centrifuge/compare/v1.1.0...v1.1.1) (2026-06-09)


### Bug Fixes

* embed migrations so deploy doesn't crash-loop (CTFG-25) ([1dc8a29](https://github.com/Einlanzerous/centrifuge/commit/1dc8a2906776c623e8fe66f196b6b1036392e7a7))
* embed migrations so deploy doesn't crash-loop (CTFG-25) ([19b1777](https://github.com/Einlanzerous/centrifuge/commit/19b17772e331d94525e65a827a051b7ad53dff4e))

## [1.1.0](https://github.com/Einlanzerous/centrifuge/compare/v1.0.0...v1.1.0) (2026-06-08)


### Features

* auto-deploy to construct-server on release (CTFG-25) ([6fb25a3](https://github.com/Einlanzerous/centrifuge/commit/6fb25a31a8c81da7e03c61451228726c6dbb2fd1))
* auto-deploy to construct-server on release (CTFG-25) ([91eea1b](https://github.com/Einlanzerous/centrifuge/commit/91eea1b058fe483379516cb1b47023c2bd9f8939))
* read-side HTTP API + RSS feed backing the UI (CTFG-26) ([14b5dd2](https://github.com/Einlanzerous/centrifuge/commit/14b5dd2556fc9434116beb84b8d90be2fc06ae49))
* read-side HTTP API + user_sessions (CTFG-26) ([138ad77](https://github.com/Einlanzerous/centrifuge/commit/138ad7711c7aa6ebe14989889003e63e3a9e8308))
* user_sessions migration + read-side DB layer (CTFG-26) ([e87acde](https://github.com/Einlanzerous/centrifuge/commit/e87acde2e7d37f60800f3c4ec509076ff77c02b7))

## 1.0.0 (2026-06-08)


### Features

* bootstrap Go module, config, logging, health endpoint (CTFG-6) ([0aa4166](https://github.com/Einlanzerous/centrifuge/commit/0aa41661c25312809318a8f6deba4f88b92f84ac))
* decoupled scoring worker (CTFG-21) ([0c2c623](https://github.com/Einlanzerous/centrifuge/commit/0c2c6239806585ff03bbb02a0e1a2cf9464dc865))
* golang-migrate integration + Makefile targets (CTFG-8) ([d28b964](https://github.com/Einlanzerous/centrifuge/commit/d28b96424d1fdaba9a8729f62f29c7aba2aacdbe))
* HTML sanitization + model-input prep (token budget) (CTFG-19) ([fd56d76](https://github.com/Einlanzerous/centrifuge/commit/fd56d76dd03da513575e085513b75f6e56728bc4))
* migration for sources + newsletters tables (CTFG-12) ([3b60412](https://github.com/Einlanzerous/centrifuge/commit/3b604120bab22db5693ab92903aeb4a6d855ef87))
* migration for stories table (CTFG-13) ([60cd6ed](https://github.com/Einlanzerous/centrifuge/commit/60cd6edc9c1026c74395c815bca677e70a4167f3))
* Ollama client for /api/generate with retries (CTFG-20) ([75a5550](https://github.com/Einlanzerous/centrifuge/commit/75a55503891c578592d46e8bd85043f0d53f45a9))
* pgx data-access layer + repositories + tests (CTFG-14) ([aac9edf](https://github.com/Einlanzerous/centrifuge/commit/aac9edf831905b05546a0692d9cf92d053e3f0cc))
* Phase 1 — schema + data-access layer (CTFG-3) ([01ca267](https://github.com/Einlanzerous/centrifuge/commit/01ca267a6107aeb6caaf6d4705fd485d369a83bc))
* Phase 2 — ingestion core + dual-format /ingest (CTFG-4) ([a5ad784](https://github.com/Einlanzerous/centrifuge/commit/a5ad7840791bb9d686ac2d766b9ec389f5472a3a))
* POST /ingest — raw RFC822 + shared-token auth (CTFG-17) ([a5c06a7](https://github.com/Einlanzerous/centrifuge/commit/a5c06a7aecf0d99a55c6f007960612e362a3c08f))
* POST /ingest/html — JSON drop for backfill / test-fire (CTFG-18) ([ee029bd](https://github.com/Einlanzerous/centrifuge/commit/ee029bd63dbc0cc96e0d9d32d110dbb5d78d6f5b))
* real-newsletter eval fixtures + prep-only mode; bump Ollama timeout ([e42c7ca](https://github.com/Einlanzerous/centrifuge/commit/e42c7ca50519c2ceb3af08fbf476dd6086066386))
* RFC822 MIME parser → InboundMessage (CTFG-16) ([5ba0588](https://github.com/Einlanzerous/centrifuge/commit/5ba0588dc49831c0607c5134cf0b560e0a691fc3))
* scoring eval harness + structured-output fix (CTFG-23) ([30877f4](https://github.com/Einlanzerous/centrifuge/commit/30877f40971a833ae9e35f8fd00123de4f0fafb2))
* scoring prompt + strict JSON validation (CTFG-22) ([eaff256](https://github.com/Einlanzerous/centrifuge/commit/eaff2565b9c74187a45e2cd6de5cf8806efdf701))
* source-agnostic ingestion core + dedupe (CTFG-15) ([cb47832](https://github.com/Einlanzerous/centrifuge/commit/cb47832786a6d205c04d9dc7f38e4274a8042442))


### Bug Fixes

* check deferred Close/Rollback error returns (errcheck) ([35787a2](https://github.com/Einlanzerous/centrifuge/commit/35787a2445385f670152d24f9286c36035c425de))
