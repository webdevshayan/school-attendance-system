import 'dart:convert';

import 'package:crypto/crypto.dart';

/// Builds the signed request headers the server's authn package expects:
///
///	X-Device-Id:      the device identity
///	X-Sync-Timestamp: RFC3339 UTC instant of signing
///	X-Sync-Signature: hex(HMAC-SHA256(secret, deviceId \n timestamp \n body))
///
/// The server enforces a 5-minute replay window around the timestamp, so the
/// wall clock must be roughly correct on a door device (NTP on Android
/// handles this; a badly skewed clock simply makes sync retry later).
Map<String, String> signedHeaders({
  required String deviceId,
  required String secret,
  required String body,
  DateTime? now,
}) {
  final timestamp = (now ?? DateTime.now().toUtc()).toIso8601String();
  final mac = Hmac(sha256, utf8.encode(secret));
  final digest = mac.convert(utf8.encode('$deviceId\n$timestamp\n$body'));
  return <String, String>{
    'Content-Type': 'application/json',
    'X-Device-Id': deviceId,
    'X-Sync-Timestamp': timestamp,
    'X-Sync-Signature': digest.toString(),
  };
}
