import 'package:flutter_test/flutter_test.dart';

import 'package:admin_app/core/db/attendance_store.dart';
import 'package:admin_app/core/export/attendance_csv.dart';

import 'fake_store.dart';

AttendanceRecord rec({
  String studentId = 's1',
  String studentName = 'Aisha',
  String status = 'in',
  DateTime? markedAt,
  String deviceId = 'door-1',
  bool synced = false,
}) =>
    AttendanceRecord(
      id: 0, // FakeStore assigns real ids on insert.
      studentId: studentId,
      studentName: studentName,
      status: status,
      markedAt: markedAt ?? DateTime.utc(2026, 11, 21, 8, 15, 30, 500),
      deviceId: deviceId,
      synced: synced,
    );

void main() {
  group('AttendanceCsv', () {
    test('header + plain row', () {
      final csv = AttendanceCsv.build([rec()]);
      expect(
        csv,
        'student_id,student_name,status,marked_at_utc,device_id,synced\n'
        's1,Aisha,in,2026-11-21T08:15:30.500Z,door-1,no\n',
      );
    });

    test('escapes commas, quotes and newlines by quoting + doubling', () {
      final csv = AttendanceCsv.build([rec(studentName: 'Lee, "Bob"\nJr')]);
      expect(csv, contains('"Lee, ""Bob""\nJr"'));
    });

    test('sorts chronologically regardless of input order', () {
      final csv = AttendanceCsv.build([
        rec(studentId: 'late', markedAt: DateTime.utc(2026, 11, 21, 10)),
        rec(studentId: 'early', markedAt: DateTime.utc(2026, 11, 20, 10)),
      ]);
      final lines = csv.trim().split('\n');
      expect(lines[1], startsWith('early'));
      expect(lines[2], startsWith('late'));
    });

    test('empty register is header only', () {
      expect(
        AttendanceCsv.build(const []),
        'student_id,student_name,status,marked_at_utc,device_id,synced\n',
      );
    });

    test('synced flag renders yes/no', () {
      expect(
        AttendanceCsv.build([rec(synced: true)]),
        contains(',yes\n'),
      );
    });
  });

  group('attendanceBetween window', () {
    test('start inclusive, end exclusive, newest first', () async {
      final store = FakeStore();
      await store.insertAttendance(
        rec(studentId: 'before', markedAt: DateTime.utc(2026, 11, 20, 9)),
      );
      await store.insertAttendance(
        rec(studentId: 'inside-late', markedAt: DateTime.utc(2026, 11, 21, 15)),
      );
      await store.insertAttendance(
        rec(studentId: 'inside-early', markedAt: DateTime.utc(2026, 11, 21, 9)),
      );
      await store.insertAttendance(
        rec(studentId: 'after', markedAt: DateTime.utc(2026, 11, 22, 9)),
      );
      final rows = await store.attendanceBetween(
        DateTime.utc(2026, 11, 21),
        DateTime.utc(2026, 11, 22),
      );
      expect(rows.map((r) => r.studentId).toList(), ['inside-late', 'inside-early']);
    });
  });
}
