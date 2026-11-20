# Parent App

Flutter app for parents. Phase 3 wires in the real experience:

1. **Nitroping push notifications** fired by `/server` the moment the child's attendance record syncs.
2. **Engaging visual dashboard** — attendance status for today, history and streaks.

## Build

Platform folders are generated at build time in CI (keeps the repo light). Locally:

```bash
flutter create . --platforms android --org dev.shayan --project-name parent_app
flutter pub get
flutter analyze
flutter test
flutter build apk --release
```
