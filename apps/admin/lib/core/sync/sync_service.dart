import 'dart:convert';

import 'package:http/http.dart' as http;

import '../config.dart';
import '../db/attendance_store.dart';
import 'request_signer.dart';

/// Pushes the offline attendance queue to POST /api/sync.
///
/// Requests are signed with the shared HMAC secret (Phase 4) and the device
/// registers itself on the server allowlist once per service instance before
/// the first drain. Rows are marked synced only on a 2xx ack; any network or
/// server error keeps the queue intact for the next cycle. Fully
/// offline-safe: without a configured URL or signing secret nothing leaves
/// the device.
class SyncService {
  SyncService({
    required AttendanceStore store,
    http.Client? client,
    this.baseUrl = AdminConfig.serverBaseUrl,
    this.signingSecret = AdminConfig.syncSigningSecret,
  })  : _store = store,
        _client = client ?? http.Client();

  final AttendanceStore _store;
  final http.Client _client;
  final String baseUrl;
  final String signingSecret;

  /// Guards against overlapping drains. The capture loop fires a sync after
  /// every mark while the periodic timer runs its own - two concurrent
  /// drains would POST the same unsynced row twice before markSynced lands,
  /// producing duplicate traffic.
  bool _draining = false;

  /// Devices whose /api/register succeeded this instance; the server's
  /// allowlist is an idempotent upsert, so re-registering is always safe.
  final Set<String> _registeredDevices = <String>{};

  /// Tries to drain the queue; returns how many rows were acknowledged.
  /// Returns 0 immediately when another drain is in flight, the server URL
  /// is still the documented placeholder, or no signing secret was baked in.
  Future<int> syncPending() async {
    if (_draining) return 0;
    if (baseUrl == AdminConfig.unconfiguredServerUrl) return 0;
    if (signingSecret.isEmpty) return 0; // unsigned builds stay offline
    _draining = true;
    try {
      final rows = await _store.pendingSync();
      var pushed = 0;
      for (final row in rows) {
        if (!await _ensureRegistered(row.deviceId)) {
          return pushed;
        }
        final body = jsonEncode(<String, dynamic>{
          'device_id': row.deviceId,
          'student_id': row.studentId,
          'status': row.status,
          'marked_at': row.markedAt.toUtc().toIso8601String(),
        });
        try {
          final response = await _client.post(
            Uri.parse('$baseUrl/api/sync'),
            headers: signedHeaders(
              deviceId: row.deviceId,
              secret: signingSecret,
              body: body,
            ),
            body: body,
          );
          if (response.statusCode == 403) {
            // The device lost its allowlist entry (fresh server DB, manual
            // removal): forget the cached registration so the next cycle
            // re-registers instead of looping on 403s forever.
            _registeredDevices.remove(row.deviceId);
            return pushed;
          }
          if (response.statusCode < 200 || response.statusCode >= 300) {
            // Server-side problem: stop draining, retry next cycle.
            return pushed;
          }
          await _store.markSynced([row.id]);
          pushed++;
        } catch (_) {
          // Offline or transport error: keep the queue, retry next cycle.
          return pushed;
        }
      }
      return pushed;
    } finally {
      _draining = false;
    }
  }

  /// Registers the device on the server allowlist (idempotent upsert).
  /// Returns false on any failure so the caller keeps the queue and retries
  /// on a later cycle.
  Future<bool> _ensureRegistered(String deviceId) async {
    if (_registeredDevices.contains(deviceId)) {
      return true;
    }
    final body = jsonEncode(<String, dynamic>{
      'device_id': deviceId,
      'label': deviceId,
    });
    try {
      final response = await _client.post(
        Uri.parse('$baseUrl/api/register'),
        headers: signedHeaders(
          deviceId: deviceId,
          secret: signingSecret,
          body: body,
        ),
        body: body,
      );
      if (response.statusCode >= 200 && response.statusCode < 300) {
        _registeredDevices.add(deviceId);
        return true;
      }
      return false;
    } catch (_) {
      return false;
    }
  }
}
