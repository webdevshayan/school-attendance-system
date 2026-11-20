# Server (Go on Vercel + Turso)

Serverless API deployed to Vercel. Phase 1 ships two zero-config Go functions under `api/`:

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/api/health` | GET | Liveness probe |
| `/api/sync` | POST | Receives attendance rows from the admin app (Phase 1: validate + ack) |

Each `api/*.go` file exports `Handler(w, r)` and is served at its own path by Vercel's Go runtime — stdlib only, no framework. `vercel.json` disables Vercel's own git deployments so the Actions pipeline (`deploy_server.yml`) is the single deploy path.

## Deploy

Automatic via `.github/workflows/deploy_server.yml` on merges to `main`. Requires the repo secrets `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`; the workflow fails loudly with instructions if any are missing.

## Phase 4 scope

- `POST /api/sync` writes validated rows to **Turso** (libSQL) using `TURSO_DATABASE_URL` + `TURSO_AUTH_TOKEN` as Vercel environment variables (server-side only, never in the repo or Actions).
- Fan-out **Nitroping** pushes to the parent app for the child's guardians.
- Device registration so only known admin devices can sync.
