# Parent App

Flutter app for parents. Phase 3 wires the real experience:

1. **Engaging visual dashboard** — today's check-in hero card (green/orange gradient), the day's event timeline, and a weekly presence summary (`X/7 days this week`).
2. **Delivery into the dashboard** — `PushHandler` accepts an attendance payload (same wire shape as `/api/sync`: `student_id`, `status`, `marked_at`, …), validates it, de-duplicates it into a local SQLite cache (sqflite, unique key on student/status/instant) and broadcasts it to the dashboard over a `Stream`. Phase 5 feeds it two ways: the **events poller** (paired installs poll `GET /api/events` with their bearer token, resumable cursor, no re-delivery) and — once the school supplies relay credentials — the live **Nitroping push**; both use the same seam, so nothing downstream changes.
3. A **debug hook** ("Simulate a push") feeds a sample payload through the real pipeline so the dashboard can be exercised before the live channel exists.

## Pairing (Phase 5)

First launch shows a pairing banner: enter the code the school gave you (the door device mints it per student on the Enroll page). `PairingService` redeems `POST /api/pair`, stores the student binding + bearer token in a local sqflite preferences store (`parent_pairing.db`), and `EventPoller` starts fetching live events every 30 s. The server never stores the token itself - only its SHA-256 - and a revoked binding (401) flips the app back to the pairing screen. The `child-1` id remains only as the debug fallback.

## Build

Platform folders are generated at build time in CI (keeps the repo light). Locally:

```bash
flutter create . --platforms android --org dev.shayan --project-name parent_app
flutter pub get
flutter analyze
flutter test
flutter build apk --release
```
