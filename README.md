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

| Component | Path | Status |
| --- | --- | --- |
| Admin App (Flutter, door-side capture) | `apps/admin` | Phase 2: camera + FaceNet recognition + libsql_dart queue; Phase 4: signed sync + device registration; Phase 5: pairing-code minting on the Enroll page |
| Parent App (Flutter, dashboard + push) | `apps/parent` | Phase 3: dashboard + local cache; Phase 5: pairing flow + live events feed into the push seam |
| Serverless API (Go on Vercel) | `server` | **Live**: `/api/health`, signed `/api/sync` (Turso + de-dup), `/api/register`, `/api/codes`, `/api/pair`, `/api/events`, fan-out seam |
| CI/CD | `.github/workflows` | Both pipelines live + Go test gate on PRs |

## Roadmap

- **Phase 1 — Scaffold & pipelines (landed):** monorepo layout, Flutter boilerplate for both apps, Go API stub on Vercel, `build_apks.yml` + `deploy_server.yml`.
- **Phase 2 — Admin core (landed):** camera preview, on-device recognition (`face_verification`, bundled FaceNet TFLite), offline attendance writes via `libsql_dart`, background sync to `/api/sync`.
- **Phase 3 — Parent core (landed):** engaging dashboard (today status, event timeline, weekly summary), local SQLite cache (sqflite, de-duplicated), transport-agnostic push handling.
- **Phase 4 — Backend integration (landed):** signed requests (HMAC-SHA256 headers + 5-minute replay window), device registration allowlist, Turso persistence with server-side de-dup, best-effort Nitroping push fan-out seam.
- **Phase 5 — Pairing & delivery (landed):** door-minted pairing codes (7-day TTL, multi-use), guardian device registry (tokens stored as SHA-256 only), `GET /api/events` cursor feed with per-student isolation — parents see attendance live via polling; switching to Nitroping push later needs only relay credentials, the `PushHandler` seam stays.
- **Next — live push & polish:** Nitroping relay credentials + parent-side push receiver (poller stays as fallback), guardian management UI (revoke/reissue codes), multi-school tenancy if needed.

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
| `SYNC_SIGNING_SECRET` | `build_apks.yml` (admin) | HMAC secret shared with the server; baked into the APK via `--dart-define`. Unset ⇒ APK builds with sync disabled |

The server also needs **Vercel environment variables** (project settings, never in Actions or the repo): `SYNC_SIGNING_SECRET` (same value), `TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`, and optionally `NITROPING_WEBHOOK_URL` + `NITROPING_API_KEY` for the push relay. Until the Turso pair is set, `/api/sync` answers 503 and the admin app keeps queueing offline.
