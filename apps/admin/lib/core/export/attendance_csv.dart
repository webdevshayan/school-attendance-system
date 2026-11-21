import '../db/attendance_store.dart';

/// Builds the school's attendance register as RFC-4180-style CSV. Pure and
/// dependency-free so it unit-tests without plugins; the share service only
/// decides where the resulting string goes.
class AttendanceCsv {
  const AttendanceCsv._();

  static const List<String> header = [
    'student_id',
    'student_name',
    'status',
    'marked_at_utc',
    'device_id',
    'synced',
  ];

  /// Quotes a field only when the data demands it; embedded quotes double up.
  static String field(Object? value) {
    final text = value.toString();
    final needsQuoting = text.contains(',') ||
        text.contains('"') ||
        text.contains('\n') ||
        text.contains('\r');
    if (!needsQuoting) return text;
    return '"${text.replaceAll('"', '""')}"';
  }

  static String row(AttendanceRecord record) => [
        field(record.studentId),
        field(record.studentName),
        field(record.status),
        field(record.markedAt.toUtc().toIso8601String()),
        field(record.deviceId),
        field(record.synced ? 'yes' : 'no'),
      ].join(',');

  /// Chronological register: oldest mark first, regardless of the order the
  /// store returned (day/window queries come newest-first).
  static String build(List<AttendanceRecord> records) {
    final sorted = [...records]
      ..sort((a, b) => a.markedAt.compareTo(b.markedAt));
    final buffer = StringBuffer('${header.join(',')}\n');
    for (final record in sorted) {
      buffer.writeln(row(record));
    }
    return buffer.toString();
  }
}
