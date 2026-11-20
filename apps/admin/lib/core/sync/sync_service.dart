import 'dart:convert';

import 'package:http/http.dart' as http;

import '../config.dart';
import '../db/attendance_store.dart';

/// Pushes the offline attendance queue to POST /api/sync.
/// Rows are marked synced only on a 2xx ack; any network or server error
/// keeps the queue intact for the next cycle. Fully offline-safe.
class SyncService {
  SyncService({
    required AttendanceStore store,
    http.Client? client,
    this.baseUrl = AdminConfig.serverBaseUrl,
  })  : _store = store,
        _client = client ?? http.Client();

  final AttendanceStore _store;
  final http.Client _client;
  final String baseUrl;

  /// Tries to drain the queue; returns how many rows were acknowledged.
  Future<int> syncPending() async {
    final rows = await _store.pendingSync();
    var pushed = 0;
    for (final row in rows) {
      try {
        final response = await _client.post(
          Uri.parse('$baseUrl/api/sync'),
          headers: const {'Content-Type': 'application/json'},
          body: jsonEncode(<String, dynamic>{
            'device_id': row.deviceId,
            'student_id': row.studentId,
            'status': row.status,
            'marked_at': row.markedAt.toUtc().toIso8601String(),
          }),
        );
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
  }
}
