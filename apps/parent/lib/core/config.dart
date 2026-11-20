/// Central knobs for the parent app.
class ChildConfig {
  ChildConfig._();

  /// Base URL of the serverless API - the live Vercel production domain.
  /// Pairing redeems codes against it and the events feed polls it.
  static const String serverBaseUrl = 'https://rgs-attendance-system.vercel.app';

  /// Sentinel for "not yet configured" parity with the admin app. The real
  /// project turned out to be rgs-*.
  static const String unconfiguredServerUrl =
      'https://school-attendance-system.vercel.app';

  /// Debug-only fallback used before this install is paired (Phase 5 wires
  /// the real pairing flow; the dashboard shows which student it follows).
  static const String childId = 'child-1';

  /// Display name shown on the dashboard hero card.
  static const String childName = 'Your child';

  /// How often the paired install polls the events feed. A live push channel
  /// (Nitroping) will replace polling when relay credentials exist; the
  /// payload flow into PushHandler stays identical either way.
  static const Duration pollInterval = Duration(seconds: 30);
}
