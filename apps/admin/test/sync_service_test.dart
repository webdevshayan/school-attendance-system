import 'dart:convert';

import 'package:admin_app/core/db/attendance_store.dart';
import 'package:admin_app/core/sync/sync_service.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'fake_store.dart';

AttendanceRecord _record(String studentId) => AttendanceRecord(
      id: 0,
      studentId: studentId,
      studentName: 'Student $studentId',
      status: 'in',
      markedAt: DateTime.utc(2026, 11, 20, 8),
      deviceId: 'door-1',
      synced: false,
    );

void main() {
  test('2xx acks mark rows synced and post the documented payload',
      () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));
    await store.insertAttendance(_record('s2'));

    final bodies = <String>[];
    final client = MockClient((request) async {
      bodies.add(request.body);
      return http.Response('{"accepted":true}', 202);
    });

    final service = SyncService(
      store: store,
      client: client,
      baseUrl: 'http://test.local',
    );

    final pushed = await service.syncPending();

    expect(pushed, 2);
    expect(await store.pendingSyncCount(), 0);

    final payload = jsonDecode(bodies.first) as Map<String, dynamic>;
    expect(payload['student_id'], 's1');
    expect(payload['status'], 'in');
    expect(payload['device_id'], 'door-1');
    expect(payload['marked_at'], '2026-11-20T08:00:00.000Z');
  });

  test('server error stops draining and keeps the queue', () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));
    await store.insertAttendance(_record('s2'));

    final client = MockClient((request) async {
      return http.Response('{"error":"boom"}', 500);
    });

    final service = SyncService(
      store: store,
      client: client,
      baseUrl: 'http://test.local',
    );

    final pushed = await service.syncPending();

    expect(pushed, 0);
    expect(await store.pendingSyncCount(), 2);
  });

  test('network errors keep the queue for the next cycle', () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));

    final client = MockClient((request) async {
      throw http.ClientException('offline');
    });

    final service = SyncService(
      store: store,
      client: client,
      baseUrl: 'http://test.local',
    );

    final pushed = await service.syncPending();

    expect(pushed, 0);
    expect(await store.pendingSyncCount(), 1);
  });

  test('concurrent syncPending calls do not double-drain the queue',
      () async {
    final store = FakeStore();
    await store.insertAttendance(_record('s1'));
    await store.insertAttendance(_record('s2'));

    var posts = 0;
    final client = MockClient((request) async {
      posts++;
      await Future<void>.delayed(const Duration(milliseconds: 10));
      return http.Response('{"accepted":true}', 202);
    });

    final service = SyncService(
      store: store,
      client: client,
      baseUrl: 'http://test.local',
    );

    // The capture loop and the periodic timer can race; the second drain
    // must bail out instead of posting the same rows again.
    final results =
        await Future.wait([service.syncPending(), service.syncPending()]);

    expect(results, [2, 0]);
    expect(posts, 2);
    expect(await store.pendingSyncCount(), 0);
  });
}
