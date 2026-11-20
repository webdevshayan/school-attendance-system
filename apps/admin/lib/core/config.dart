/// Central knobs for the door-side admin app.
///
/// Everything here is a constant on purpose: a door device runs unattended,
/// so behavior must be predictable. Tunables become persisted settings later
/// if the school asks for them.
class AdminConfig {
  AdminConfig._();

  /// Sentinel for "not yet configured". While [serverBaseUrl] equals this,
  /// SyncService skips the network and the UI shows a banner; attendance
  /// keeps queueing offline either way. (The placeholder was the guessed
  /// domain from Phase 1; the real project turned out to be rgs-*.)
  static const String unconfiguredServerUrl =
      'https://school-attendance-system.vercel.app';

  /// Base URL of the serverless API - the live Vercel production domain
  /// (project linked; GET /api/health verified 2026-11-20). Phase 4 makes
  /// the server persist to Turso.
  static const String serverBaseUrl =
      'https://rgs-attendance-system.vercel.app';

  /// True once [serverBaseUrl] has been pointed at the real deployment.
  static bool get isServerConfigured =>
      serverBaseUrl != unconfiguredServerUrl;

  /// Identifier stamped on every attendance row sent to /api/sync so the
  /// server can tell doors apart (device registration allowlist, Phase 4).
  static const String deviceId = 'door-1';

  /// HMAC secret for signed /api/sync + /api/register requests, shared with
  /// the server's SYNC_SIGNING_SECRET. Injected at compile time via
  /// --dart-define=SYNC_SIGNING_SECRET=... (CI injects it from the Actions
  /// secret; it never lives in the repo). Empty on local debug builds, which
  /// keeps sync disabled - attendance still queues offline either way.
  static const String syncSigningSecret =
      String.fromEnvironment('SYNC_SIGNING_SECRET');

  /// True when the app may talk to the server: real base URL AND a signing
  /// secret. [isServerConfigured] stays URL-based for the banner; [canSync]
  /// gates the actual network calls.
  static bool get canSync =>
      isServerConfigured && syncSigningSecret.isNotEmpty;

  /// Cosine-similarity threshold for face matches (face_verification default).
  static const double matchThreshold = 0.70;

  /// Minimum time between two attendance marks for the same student.
  static const Duration perStudentCooldown = Duration(minutes: 3);

  /// How often the door loop captures and verifies a photo.
  static const Duration captureInterval = Duration(seconds: 2);

  /// How often the background sync pushes the offline queue.
  static const Duration syncInterval = Duration(seconds: 30);
}
