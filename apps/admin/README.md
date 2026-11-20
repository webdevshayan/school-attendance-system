# Admin App (door-side capture)

The door-side Flutter app. Phase 2 wires the full local pipeline; nothing here needs the network to mark attendance.

## How it works

1. **Camera preview** pointed at the entrance (back camera, medium resolution).
2. Every 2 s the app snaps a photo and runs **on-device recognition** through `face_verification` — a FaceNet TFLite model bundled inside the plugin (no downloads), verified in a background isolate, cosine threshold 0.70.
3. A match becomes an attendance row in **local SQLite via `libsql_dart`** (`LibsqlClient.local`), gated by:
   - a per-student cooldown (3 min) so a lingering face isn't double-marked,
   - one mark per status per UTC day (a second `in` the same day is ignored; `in` and `out` are independent).
4. **Background sync** pushes unsynced rows to `POST /api/sync` every 30 s; rows are flagged synced only on a 2xx ack, so the queue survives offline periods intact.

Degraded states are explicit: if the face engine, the local database or the camera is unavailable, the home screen shows exactly what failed with a Retry button instead of crashing.

## Enroll

`Enroll` (FAB): name (+ optional id) -> camera snap -> face sample (`door_enrollment` slot, replace-on-recapture) + students row. Removing a student deletes both their local row and their face sample.

## Configuration

`lib/core/config.dart`: server base URL (placeholder until the Vercel project is linked — swap in the real production domain), device id (`door-1`), match threshold, cooldown, capture and sync intervals.

## Build

Platform folders are generated at build time in CI, which also patches in the CAMERA/INTERNET permissions and pins `kotlin.jvm.target.validation.mode=warning`. Locally:

```bash
flutter create . --platforms android --org dev.shayan --project-name admin_app
# patch android/app/src/main/AndroidManifest.xml with CAMERA + INTERNET permissions
flutter pub get
flutter analyze
flutter test
flutter build apk --release
```

`libsql_dart` ships a Rust core via Dart native assets; building locally needs a Rust toolchain (`rustup`), which CI provides.
