# Parent App

Flutter app for parents. Phase 3 wires the real experience:

1. **Engaging visual dashboard** — today's check-in hero card (green/orange gradient), the day's event timeline, and a weekly presence summary (`X/7 days this week`).
2. **Transport-agnostic push handling** — `PushHandler` accepts an attendance payload (same wire shape as `/api/sync`: `student_id`, `status`, `marked_at`, …), validates it, de-duplicates it into a local SQLite cache (sqflite, unique key on student/status/instant) and broadcasts it to the dashboard over a `Stream`. Phase 4 connects this to the server's fan-out (Nitroping) — no app-side change needed, the handler is the integration seam.
3. A **debug hook** ("Simulate a push") feeds a sample payload through the real pipeline so the dashboard can be exercised before the live channel exists.

The child identity is a placeholder (`ChildConfig.childId = 'child-1'`) until Phase 4 pairs the install with a real child via device registration.

## Build

Platform folders are generated at build time in CI (keeps the repo light). Locally:

```bash
flutter create . --platforms android --org dev.shayan --project-name parent_app
flutter pub get
flutter analyze
flutter test
flutter build apk --release
```
