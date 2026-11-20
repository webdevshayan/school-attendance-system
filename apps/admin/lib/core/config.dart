/// Central knobs for the door-side admin app.
///
/// Everything here is a constant on purpose: a door device runs unattended,
/// so behavior must be predictable. Tunables become persisted settings later
/// if the school asks for them.
class AdminConfig {
  AdminConfig._();

  /// Placeholder until the Vercel project is linked. While the base URL
  /// equals this, SyncService skips the network and the UI shows a banner;
  /// attendance keeps queueing offline either way. Phase 4 makes the server
  /// persist to Turso. Replace [serverBaseUrl] with the real production
  /// domain once `vercel link` is done.
  static const String unconfiguredServerUrl =
      'https://school-attendance-system.vercel.app';

  /// Base URL of the serverless API.
  static const String serverBaseUrl = unconfiguredServerUrl;

  /// True once [serverBaseUrl] has been pointed at the real deployment.
  static bool get isServerConfigured =>
      serverBaseUrl != unconfiguredServerUrl;

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
