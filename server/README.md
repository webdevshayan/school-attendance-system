# Server (Go on Vercel + Turso)

Serverless API deployed to Vercel. Phase 4 is live: signed requests, device allowlist, Turso persistence and best-effort push fan-out.

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/api/health` | GET | Liveness + which integrations are configured (`signing`, `turso`, `fanout` fields) |
| `/api/register` | POST | Adds a signed device to the sync allowlist (idempotent upsert) |
| `/api/sync` | POST | Validates, de-duplicates and persists one attendance row; fans out a push |

## Wire contract (both POST endpoints)

Requests are authenticated with three headers; the signature covers the exact bytes of the body:

```
X-Device-Id:      <device identity, e.g. door-1>
X-Sync-Timestamp: <RFC3339 UTC instant of signing>
X-Sync-Signature: <hex HMAC-SHA256(secret, deviceId \n timestamp \n body)>
```

- Timestamps drifting more than **5 minutes** from server time are rejected (replay window).
- The body's `device_id` must match the signed `X-Device-Id` header.
- `/api/sync` requires a registered device (`403` otherwise); `/api/register` upserts it.
- Rows are de-duplicated server-side on `(device_id, student_id, status, marked_at)` — a lost-ack retry cannot double-record attendance. The response reports `"duplicate": true` for a repeat.
- Missing/invalid configuration answers `503` with the exact missing environment variable names; storage failures answer `502`. The admin app treats any non-2xx as "keep the queue, retry later".

## Environment variables (Vercel project settings, server-side only)

| Variable | Required | Purpose |
| --- | --- | --- |
| `SYNC_SIGNING_SECRET` | yes | HMAC secret shared with the admin APK (which gets it at compile time via `--dart-define`) |
| `TURSO_DATABASE_URL` | yes | libSQL database URL |
| `TURSO_AUTH_TOKEN` | yes | libSQL auth token |
| `NITROPING_WEBHOOK_URL` | optional | Push relay endpoint; fan-out is skipped when unset |
| `NITROPING_API_KEY` | optional | Push relay bearer token |

## Layout & the Vercel Go runtime contract

All logic lives in `internal/` (authn, turso, store, fanout, handlers) — `go test ./internal/...` covers it in CI. The `api/*.go` files are thin adapters forming **one `package handler`**, each exporting a **distinctly named** function with the standard signature (`Health`, `Sync`, `Register`). In module mode (a `go.mod` present) the runtime compiles the whole package per route, rewrites the `go.mod` module line to the package name, and renames the entrypoint's handler (found by *signature*, any exported name) during that route's build — so per-file exports must stay distinct and `internal/handlers` calls must stay qualified. Stdlib only, no framework; Turso is reached over its HTTP pipeline API (`internal/turso`) because the official SDK needs a native libsql core this runtime can't run.

**Framework Preset on Vercel must stay "Other"** — a Golang app preset makes the platform look for `main.go` and fail with "No Go entrypoint found". `deploy_server.yml` self-heals this setting before every deploy.

`vercel.json` disables Vercel's own git deployments so the Actions pipeline (`deploy_server.yml`) is the single deploy path.

## Deploy

Automatic via `.github/workflows/deploy_server.yml` on merges to `main` (PRs run the Go test gate first: gofmt + vet + `go test ./internal/...`). Requires the repo secrets `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`; the workflow fails loudly with instructions if any are missing.

## Next: parent fan-out channel

`internal/fanout` posts events (wire shape identical to `/api/sync`) to the Nitroping relay when `NITROPING_WEBHOOK_URL` is set; the parent app's `PushHandler` is already the receiving seam, so wiring the live channel needs no app change — only relay credentials and (if routing by child) a guardian mapping.
