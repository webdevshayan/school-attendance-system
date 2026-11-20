import 'package:flutter_test/flutter_test.dart';

import 'package:parent_app/core/models/attendance_event.dart';

void main() {
  test('round-trips through json', () {
    final at = DateTime.utc(2026, 11, 20, 8, 15);
    final event = AttendanceEvent(
      studentId: 's1',
      studentName: 'Ali',
      status: 'in',
      markedAt: at,
      deviceId: 'door-1',
    );

    final parsed = AttendanceEvent.fromJson(event.toJson());

    expect(parsed.studentId, 's1');
    expect(parsed.studentName, 'Ali');
    expect(parsed.status, 'in');
    expect(parsed.markedAt, at);
    expect(parsed.deviceId, 'door-1');
  });

  test('parses the wire payload documented in /api/sync', () {
    final event = AttendanceEvent.fromJson(<String, dynamic>{
      'student_id': 's1',
      'student_name': 'Ali',
      'status': 'out',
      'marked_at': '2026-11-20T08:15:00.000Z',
      'device_id': 'door-1',
    });

    expect(event.status, 'out');
    expect(event.markedAt.isUtc, isTrue);
    expect(event.studentName, 'Ali');
  });

  test('tolerates missing optional fields', () {
    final event = AttendanceEvent.fromJson(<String, dynamic>{
      'student_id': 's1',
      'marked_at': '2026-11-20T08:15:00.000Z',
    });

    expect(event.status, 'in');
    expect(event.studentName, 's1');
    expect(event.deviceId, '');
  });

  test('rejects a malformed timestamp by throwing FormatException', () {
    expect(
      () => AttendanceEvent.fromJson(<String, dynamic>{
        'student_id': 's1',
        'marked_at': 'not-a-date',
      }),
      throwsFormatException,
    );
  });
}
