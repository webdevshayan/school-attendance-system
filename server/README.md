# Server (Go on Vercel + Turso)

Serverless API deployed to Vercel. Phase 4 is live: signed requests, device allowlist and Turso persistence. Phase 5 adds the pairing layer: door-minted codes, a guardian device registry and the parent events feed — which IS the parent delivery channel (free cursor polling, no relay dependency).

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/api/health` | GET | Liveness + which integrations are configured (`signing`, `turso`, `push` fields) |
| `/api/register` | POST | Adds a signed device to the sync allowlist (idempotent upsert) |
| `/api/sync` | POST | Validates, de-duplicates and persists one attendance row; parents receive it on their next events poll |
| `/api/codes` | POST | Mints a parent-pairing code for a student (signed, registered door devices only) |
| `/api/pair` | POST | Redeems a code: binds a parent install to the student, returns its bearer token once. Rate-limited per client IP (default 10/min, `PAIR_RATE_LIMIT`; `429` + `Retry-After` when exceeded) |
| `/api/events` | GET | Cursor feed of a paired student's attendance rows (Bearer token; 100 rows max) |

## Pairing & guardian registry (Phase 5)

- **Minting**: a registered door device HMAC-signs `{student_id}` to `POST /api/codes`; the server answers `{code, student_id, expires_at}` (8 chars, lookalike-free alphabet, 7-day TTL, multi-use so both guardians pair from one code).
- **Redemption**: the parent app posts `{code, label, platform}` to `POST /api/pair` — the code itself is the credential. Unknown → `400`; expired → `410`. Success returns `{student_id, token}` **exactly once**; the server stores only the token's SHA-256 (`parent_devices.token_hash`).
- **Feed**: `GET /api/events?since=<id>` with `Authorization: Bearer <token>` returns the paired student's rows oldest-first with a `cursor`; payloads reuse the `/api/sync` wire shape (`device_id`, `student_id`, `status`, `marked_at`), so the parent app's `PushHandler` consumes them unchanged. Rows for other students never cross the boundary.
- **Abuse resistance**: `/api/pair` is rate-limited per client IP (default 10 attempts/min; `PAIR_RATE_LIMIT` tunes or disables it; breaches answer `429` + `Retry-After`). The limiter is an in-memory fixed window per serverless instance — no cross-instance state exists on this runtime — so it makes grinding slow and observable; the code's entropy and 7-day TTL remain the real bound.
- The poll is the official delivery channel by design: free, serverless-friendly, and with zero third-party relay credentials to rotate.


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
| `PAIR_RATE_LIMIT` | optional | `/api/pair` attempts per client IP per minute (default `10`, `0` disables the limiter) |

## Layout & the Vercel Go runtime contract

All logic lives in `pkg/` (authn, turso, store, handlers) — `go test ./pkg/...` covers it in CI. Two serving shapes share it, both dispatching through `pkg/routes`:

- **Primary — Framework Preset `Go`**: Vercel detects the project-root `go.mod` + the `main.go` entrypoint and builds **one server binary**. The entrypoint contract ([docs/functions/runtimes/go](https://vercel.com/docs/functions/runtimes/go)) is strict: `package main`, listening on the `PORT` environment variable. `main.go` is exactly that — which also makes it the local dev server (`go run .` from `server/`).
- **Fallback — Framework Preset `Other`**: the `api/*.go` files are thin adapters forming **one `package handler`**, each exporting a **distinctly named** function with the standard signature (`Health`, `Sync`, `Register`). In module mode the runtime compiles the package per route and renames the handler it matches by *signature* — per-file exports must stay distinct and `pkg/handlers` calls stay qualified. Stdlib only, no framework; Turso is reached over its HTTP pipeline API (`pkg/turso`) because the official SDK needs a native libsql core this runtime can't run.

`deploy_server.yml` sets the preset to `Go` before every deploy, **verifies the deployment on a preview URL with the real credentials and only then promotes it to production**; an unverifiable Go-framework build automatically falls back to `Other`. Production is never touched by an unverified build — the 2026-11-20/21 incidents ("No Go entrypoint found", then `fork/exec ./user-server: exec format error` at invocation) came from unverified deployments going live, which this pipeline makes impossible.

`vercel.json` disables Vercel's own git deployments so the Actions pipeline (`deploy_server.yml`) is the single deploy path.

## Deploy

Automatic via `.github/workflows/deploy_server.yml` on merges to `main` (PRs run the Go test gate first: gofmt + vet + `go test ./pkg/...`). Requires the repo secrets `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`; the workflow fails loudly with instructions if any are missing.

## Parent delivery channel

Parents poll `GET /api/events?since=<cursor>` with their pairing bearer token (30 s cadence in the parent app). The cursor feed is the live channel by design — free, serverless-friendly, no relay credentials — and the parent app's `PushHandler` stays the transport-agnostic receiving seam, so any future transport (websocket, FCM, …) could feed it unchanged.
