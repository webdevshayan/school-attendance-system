# Admin App (door-side capture)

Flutter app that lives at the school door. Phase 2 wires in the real pipeline:

1. **Camera preview** pointed at the entrance.
2. **On-device face recognition** via `face_verification` (TFLite) — no network round trip, so marking is low-latency and works fully offline.
3. **Local SQLite writes** via `libsql_dart` — every match becomes an attendance row in the offline queue.
4. **Opportunistic sync** — when connectivity returns, queued rows go to `POST /api/sync` on `/server`, which persists to Turso and pushes to the parent app.

## Build

Platform folders are generated at build time in CI (keeps the repo light). Locally:

```bash
flutter create . --platforms android --org dev.shayan --project-name admin_app
flutter pub get
flutter analyze
flutter test
flutter build apk --release
```
