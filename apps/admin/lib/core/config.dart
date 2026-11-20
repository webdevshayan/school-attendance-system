/// Central knobs for the door-side admin app.
///
/// Everything here is a constant on purpose: a door device runs unattended,
/// so behavior must be predictable. Tunables become persisted settings later
/// if the school asks for them.
class AdminConfig {
  AdminConfig._();

  /// Base URL of the serverless API. The Phase 1 stub already validates and
  /// acknowledges these payloads; Phase 4 makes it persist to Turso.
  /// Replace once the Vercel project is linked and has its production domain.
  static const String serverBaseUrl =
      'https://school-attendance-system.vercel.app';

  /// Identifier stamped on every attendance row sent to /api/sync so the
  /// server (Phase 4 device registration) can tell doors apart.
  static const String deviceId = 'door-1';

  /// Cosine-similarity threshold for face matches (face_verification default).
  static const double matchThreshold = 0.70;

  /// Minimum time between two attendance marks for the same student.
  static const Duration perStudentCooldown = Duration(minutes: 3);

  /// How often the door loop captures and verifies a photo.
  static const Duration captureInterval = Duration(seconds: 2);

  /// How often the background sync pushes the offline queue.
  static const Duration syncInterval = Duration(seconds: 30);
}
