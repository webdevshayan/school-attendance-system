# school-attendance-system

A 100% offline-first, low-latency attendance system for schools. A door-side **Admin App** recognises faces on-device and marks attendance into a local SQLite database; queued records sync to Turso Cloud the moment connectivity returns, and every sync fires a push notification to the child's **Parent App**.

```
Face at door ──▶ /apps/admin (Flutter)
                 ├─ TFLite face match on-device   (low latency, works offline)
                 ├─ attendance row → local SQLite (libsql_dart)
                 └─ online? → POST /api/sync ──▶ /server (Go, Vercel)
                                                 ├─ writes to Turso Cloud
                                                 └─ triggers push (Nitroping)
                                                        ──▶ /apps/parent (Flutter)
                                                             visual dashboard
```

| Component | Path | Phase 1 status |
| --- | --- | --- |
| Admin App (Flutter, door-side capture) | `apps/admin` | Phase 2: camera + FaceNet recognition + libsql_dart queue + sync |
| Parent App (Flutter, dashboard + push) | `apps/parent` | Buildable scaffold, smoke test green |
| Serverless API (Go on Vercel) | `server` | `/api/health` live, `/api/sync` stub |
| CI/CD | `.github/workflows` | Both pipelines live |

## Roadmap

- **Phase 1 — Scaffold & pipelines (this PR):** monorepo layout, Flutter boilerplate for both apps, Go API stub on Vercel, `build_apks.yml` + `deploy_server.yml`.
- **Phase 2 — Admin core (landed):** camera preview, on-device recognition (`face_verification`, bundled FaceNet TFLite), offline attendance writes via `libsql_dart`, background sync to `/api/sync`.
- **Phase 3 — Parent core:** engaging visual dashboard, Nitroping push handling.
- **Phase 4 — Backend integration:** Turso persistence in `/api/sync` with device registration + signed requests (the endpoint must not persist unauthenticated writes), push fan-out.

## CI/CD & storage hygiene

- **`build_apks.yml`** builds **both** Flutter apps on every PR (Java 17 + stable Flutter), runs `flutter analyze` and the smoke tests, uploads release APKs with **`retention-days: 1`**, and a final job purges any artifact older than 24 h via the API. No heavy file ever lives in the repo or on the runners beyond a single build. The admin job additionally provisions a Rust toolchain (`libsql_dart` ships a native core via Dart native assets) and patches the generated Android manifest with CAMERA/INTERNET permissions.
- **`deploy_server.yml`** deploys `server/` to Vercel production on merges to `main`; it fails loudly if the Vercel secrets are missing.
- Flutter `android/` platform folders are intentionally **not committed** — CI runs `flutter create . --platforms android` so every build is generated against the exact Flutter version that compiles it, then patches in the CAMERA/INTERNET permissions, gradle tolerances and the lint-task disable (decided in Phase 2: keep them generated to stay light; revisit if the patch step grows).
- No tokens, keys or `.env` files are ever committed — the root `.gitignore` blocks them.

## Required secrets

`Settings → Secrets and variables → Actions`:

| Secret | Used by | Notes |
| --- | --- | --- |
| `VERCEL_TOKEN` | `deploy_server.yml` | Vercel account token |
| `VERCEL_ORG_ID` | `deploy_server.yml` | from `vercel link` |
| `VERCEL_PROJECT_ID` | `deploy_server.yml` | from `vercel link` |

Phase 4 adds `TURSO_DATABASE_URL` / `TURSO_AUTH_TOKEN` as **Vercel environment variables** (server-side only, never in Actions or the repo).
