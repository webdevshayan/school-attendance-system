import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:parent_app/core/db/event_store.dart';
import 'package:parent_app/core/models/attendance_event.dart';
import 'package:parent_app/core/pairing/event_poller.dart';
import 'package:parent_app/core/pairing/pairing_preferences.dart';
import 'package:parent_app/core/pairing/pairing_service.dart';
import 'package:parent_app/core/push/push_handler.dart';

class _RecordingStore implements EventStore {
  final List<AttendanceEvent> added = [];

  @override
  Future<void> addEvent(AttendanceEvent event) async => added.add(event);
}

/// Builds a paired service over in-memory prefs: [token]/[student] seeded
/// directly, exactly as a successful /api/pair would leave them.
PairingService pairedService(PairingPreferences prefs) {
  prefs.write(PairingKeys.studentId, 's1');
  prefs.write(PairingKeys.token, 'f' * 64);
  return PairingService(
    prefs: prefs,
    client: MockClient((request) async {
      throw StateError('pairing service must not hit the network in poller tests');
    }),
    baseUrl: 'http://test.local',
  );
}

void main() {
  test('pollOnce delivers events into PushHandler and advances the cursor',
      () async {
    final store = _RecordingStore();
    final handler = PushHandler(store: store);
    final prefs = InMemoryPairingPreferences();
    final pairing = pairedService(prefs);

    var polls = 0;
    final client = MockClient((request) async {
      polls++;
      expect(request.url.path, '/api/events');
      expect(request.url.queryParameters['since'], '0');
      expect(request.headers['Authorization'], 'Bearer ${'f' * 64}');
      return http.Response(
        '{"events":['
        '{"id":1,"device_id":"door-1","student_id":"s1","status":"in","marked_at":"2026-11-20T08:00:00.000Z"},'
        '{"id":2,"device_id":"door-1","student_id":"s1","status":"out","marked_at":"2026-11-20T15:00:00.000Z"}'
        '],"cursor":2}',
        200,
      );
    });
    final poller = EventPoller(
      pairing: pairing,
      push: handler,
      client: client,
      baseUrl: 'http://test.local',
    );

    await poller.pollOnce();

    expect(store.added, hasLength(2));
    expect(store.added.first.status, 'in');
    expect(store.added.last.status, 'out');
    expect(await prefs.read(PairingKeys.cursor), '2');
    expect(poller.lastError, isNull);
    expect(polls, 1);
  });

  test('the cursor resumes where the last poll stopped', () async {
    final store = _RecordingStore();
    final handler = PushHandler(store: store);
    final prefs = InMemoryPairingPreferences();
    final pairing = pairedService(prefs);
    await prefs.write(PairingKeys.cursor, '7');

    final seenSince = <String>[];
    final client = MockClient((request) async {
      seenSince.add(request.url.queryParameters['since'] ?? '');
      return http.Response('{"events":[],"cursor":7}', 200);
    });
    final poller = EventPoller(
      pairing: pairing,
      push: handler,
      client: client,
      baseUrl: 'http://test.local',
    );

    await poller.pollOnce();

    expect(seenSince, ['7']);
    expect(store.added, isEmpty,
        reason: 'events below the cursor must not be re-delivered');
  });

  test('a revoked binding unpairs, notifies and stops delivering', () async {
    final store = _RecordingStore();
    final handler = PushHandler(store: store);
    final prefs = InMemoryPairingPreferences();
    final pairing = pairedService(prefs);
    var revokedCallbacks = 0;

    final client = MockClient((request) async {
      return http.Response('{"error":"invalid or revoked token"}', 401);
    });
    final poller = EventPoller(
      pairing: pairing,
      push: handler,
      client: client,
      baseUrl: 'http://test.local',
      onRevoked: () => revokedCallbacks++,
    );

    await poller.pollOnce();

    expect(revokedCallbacks, 1);
    expect(await pairing.isPaired(), isFalse,
        reason: 'binding must be forgotten so the UI returns to pairing');
    expect(store.added, isEmpty);
    expect(poller.lastError, contains('revoked'));
  });

  test('server errors surface in lastError without touching the cursor',
      () async {
    final store = _RecordingStore();
    final handler = PushHandler(store: store);
    final prefs = InMemoryPairingPreferences();
    final pairing = pairedService(prefs);

    final client = MockClient((request) async {
      return http.Response('{"error":"boom"}', 500);
    });
    final poller = EventPoller(
      pairing: pairing,
      push: handler,
      client: client,
      baseUrl: 'http://test.local',
    );

    await poller.pollOnce();

    expect(poller.lastError, contains('500'));
    expect(await prefs.read(PairingKeys.cursor), isNull);
    expect(store.added, isEmpty);
  });

  test('an unpaired install never hits the network', () async {
    final store = _RecordingStore();
    final handler = PushHandler(store: store);
    var polls = 0;
    final client = MockClient((request) async {
      polls++;
      return http.Response('{"events":[],"cursor":0}', 200);
    });
    final poller = EventPoller(
      pairing: PairingService(
        prefs: InMemoryPairingPreferences(),
        client: client,
        baseUrl: 'http://test.local',
      ),
      push: handler,
      client: client,
      baseUrl: 'http://test.local',
    );

    await poller.pollOnce();

    expect(polls, 0);
    expect(store.added, isEmpty);
  });
}
