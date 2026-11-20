import 'dart:convert';

import 'package:admin_app/core/db/attendance_store.dart';
import 'package:admin_app/core/sync/sync_service.dart';
import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'fake_store.dart';

const testSecret = 'phase4-test-secret';

AttendanceRecord _record(String studentId) => AttendanceRecord(
      id: 0,
      studentId: studentId,
      studentName: 'Student $studentId',
      status: 'in',
      markedAt: DateTime.utc(2026, 11, 20, 8),
      deviceId: 'door-1',
      synced: false,
    );

/// Recomputes the signature the server's authn package expects, so the tests
/// verify the wire format rather than trusting the client blindly.
String _expectedSignature(String deviceId, String timestamp, String body) {
  final mac = Hmac(sha256, utf8.encode(testSecret));
  return mac.convert(utf8.encode('$deviceId\n$timestamp\n$body')).toString();
}

/// MockClient that answers /api/register with 200 and records every /api/sync.
/// Returns the sync bodies, sync headers and the register call count.
class RecordingClient {
  final bodies = <String>[];
  final headers = <Map<String, String>>[];
  int registerCalls = 0;
  int registerStatus = 200;
  int syncStatus = 202;
  Object? syncError;

  http.Client build() => MockClient((request) async {
        if (request.url.path.endsWith('/api/register')) {
          registerCalls++;
          return http.Response('{"registered":true}', registerStatus);
        }
        if (syncError != null) {
          throw syncError!;
        }
        bodies.add(request.body);
        headers.add(request.headers);
        return http.Response('{"accepted":true,"stored":true}', syncStatus);
      });
}

void main() {
  test('2xx acks mark rows synced and post the documented signed payload',
      () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));
    await store.insertAttendance(_record('s2'));

    final recording = RecordingClient();
    final service = SyncService(
      store: store,
      client: recording.build(),
      baseUrl: 'http://test.local',
      signingSecret: testSecret,
    );

    final pushed = await service.syncPending();

    expect(pushed, 2);
    expect(await store.pendingSyncCount(), 0);

    // The device registered itself once, then posted both rows.
    expect(recording.registerCalls, 1);
    expect(recording.bodies.length, 2);

    final payload = jsonDecode(recording.bodies.first) as Map<String, dynamic>;
    expect(payload['student_id'], 's1');
    expect(payload['status'], 'in');
    expect(payload['device_id'], 'door-1');
    expect(payload['marked_at'], '2026-11-20T08:00:00.000Z');

    // The signature must match the server-side recomputation exactly:
    // HMAC-SHA256(secret, deviceId \n timestamp \n body).
    final h = recording.headers.first;
    expect(h['x-device-id'], 'door-1');
    expect(h['x-sync-signature'],
        _expectedSignature('door-1', h['x-sync-timestamp']!, recording.bodies.first));
  });

  test('server error stops draining and keeps the queue', () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));
    await store.insertAttendance(_record('s2'));

    final recording = RecordingClient()..syncStatus = 500;
    final service = SyncService(
      store: store,
      client: recording.build(),
      baseUrl: 'http://test.local',
      signingSecret: testSecret,
    );

    final pushed = await service.syncPending();

    expect(pushed, 0);
    expect(await store.pendingSyncCount(), 2);
    expect(recording.bodies.length, 1, reason: 'first 500 stops the drain');
  });

  test('registration failure keeps the queue and skips the sync post',
      () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));

    final recording = RecordingClient()..registerStatus = 503;
    final service = SyncService(
      store: store,
      client: recording.build(),
      baseUrl: 'http://test.local',
      signingSecret: testSecret,
    );

    final pushed = await service.syncPending();

    expect(pushed, 0);
    expect(await store.pendingSyncCount(), 1);
    expect(recording.registerCalls, 1);
    expect(recording.bodies, isEmpty,
        reason: 'an unregistered device must not post rows');
  });

  test('network errors keep the queue for the next cycle', () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));

    final recording = RecordingClient()..syncError = http.ClientException('offline');
    final service = SyncService(
      store: store,
      client: recording.build(),
      baseUrl: 'http://test.local',
      signingSecret: testSecret,
    );

    final pushed = await service.syncPending();

    expect(pushed, 0);
    expect(await store.pendingSyncCount(), 1);
  });

  test('403 clears the cached registration so the next cycle re-registers',
      () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));

    final recording = RecordingClient()..syncStatus = 403;
    final service = SyncService(
      store: store,
      client: recording.build(),
      baseUrl: 'http://test.local',
      signingSecret: testSecret,
    );

    expect(await service.syncPending(), 0);
    expect(await service.syncPending(), 0);
    expect(recording.registerCalls, 2,
        reason: 'after a 403 the device must re-register on the next cycle');
  });

  test('an unsigned build (no baked-in secret) never touches the network',
      () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));

    final recording = RecordingClient();
    final service = SyncService(
      store: store,
      client: recording.build(),
      baseUrl: 'http://test.local',
      signingSecret: '', // local debug builds have no --dart-define
    );

    final pushed = await service.syncPending();

    expect(pushed, 0);
    expect(await store.pendingSyncCount(), 1);
    expect(recording.registerCalls, 0);
    expect(recording.bodies, isEmpty);
  });

  test('concurrent syncPending calls do not double-drain the queue',
      () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));
    await store.insertAttendance(_record('s2'));

    var syncPosts = 0;
    final client = MockClient((request) async {
      if (request.url.path.endsWith('/api/register')) {
        return http.Response('{"registered":true}', 200);
      }
      syncPosts++;
      await Future<void>.delayed(const Duration(milliseconds: 10));
      return http.Response('{"accepted":true}', 202);
    });

    final service = SyncService(
      store: store,
      client: client,
      baseUrl: 'http://test.local',
      signingSecret: testSecret,
    );

    // The capture loop and the periodic timer can race; the second drain
    // must bail out instead of posting the same rows again.
    final results =
        await Future.wait([service.syncPending(), service.syncPending()]);

    expect(results, [2, 0]);
    expect(syncPosts, 2);
    expect(await store.pendingSyncCount(), 0);
  });
}
