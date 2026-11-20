import 'package:flutter_test/flutter_test.dart';

import 'package:parent_app/core/db/event_store.dart';
import 'package:parent_app/core/models/attendance_event.dart';
import 'package:parent_app/core/push/push_handler.dart';

class _FakeStore implements EventStore {
  final List<AttendanceEvent> added = [];

  @override
  Future<void> addEvent(AttendanceEvent event) async => added.add(event);
}

void main() {
  test('accepts a valid payload, stores it and broadcasts the event',
      () async {
    final store = _FakeStore();
    final handler = PushHandler(store: store);
    final seen = <AttendanceEvent>[];
    final sub = handler.events.listen(seen.add);

    final ok = await handler.handle(<String, dynamic>{
      'student_id': 's1',
      'student_name': 'Ali',
      'status': 'in',
      'marked_at': '2026-11-20T08:15:00.000Z',
      'device_id': 'door-1',
    });

    expect(ok, isTrue);
    expect(store.added, hasLength(1));
    expect(store.added.first.status, 'in');

    await Future<void>.delayed(Duration.zero);
    expect(seen, hasLength(1));

    await sub.cancel();
    await handler.dispose();
  });

  test('rejects malformed or invalid payloads without storing', () async {
    final store = _FakeStore();
    final handler = PushHandler(store: store);

    expect(await handler.handle(<String, dynamic>{'student_id': 's1'}),
        isFalse);
    expect(
      await handler.handle(<String, dynamic>{
        'student_id': 's1',
        'marked_at': 'not-a-date',
      }),
      isFalse,
    );
    expect(
      await handler.handle(<String, dynamic>{
        'student_id': '',
        'marked_at': '2026-11-20T08:15:00.000Z',
      }),
      isFalse,
    );
    expect(
      await handler.handle(<String, dynamic>{
        'student_id': 's1',
        'status': 'maybe',
        'marked_at': '2026-11-20T08:15:00.000Z',
      }),
      isFalse,
    );

    expect(store.added, isEmpty);
    await handler.dispose();
  });
}
