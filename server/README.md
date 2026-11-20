# Server (Go on Vercel + Turso)

Serverless API deployed to Vercel. Phase 1 ships two zero-config Go functions under `api/`:

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/api/health` | GET | Liveness probe |
| `/api/sync` | POST | Receives attendance rows from the admin app (Phase 1: validate + ack) |

The `api/*.go` files form **one `package handler`**, each exporting `Handler(w, r)`, served at its own path (`/api/health`, `/api/sync`) by Vercel's Go runtime in module mode: with a `go.mod` present, the runtime requires the `api/` directory to be a single importable, non-`main` package (its builder rewrites the `go.mod` module line to the package name and compiles the whole package with a generated bootstrap; the entrypoint's `Handler` is renamed per build, so the per-file same-name exports are fine). Stdlib only, no framework. `vercel.json` disables Vercel's own git deployments so the Actions pipeline (`deploy_server.yml`) is the single deploy path.

## Deploy

Automatic via `.github/workflows/deploy_server.yml` on merges to `main`. Requires the repo secrets `VERCEL_TOKEN`, `VERCEL_ORG_ID`, `VERCEL_PROJECT_ID`; the workflow fails loudly with instructions if any are missing.

## Phase 4 scope

- `POST /api/sync` writes validated rows to **Turso** (libSQL) using `TURSO_DATABASE_URL` + `TURSO_AUTH_TOKEN` as Vercel environment variables (server-side only, never in the repo or Actions).
- Fan-out **Nitroping** pushes to the parent app for the child's guardians.
- Device registration so only known admin devices can sync.
