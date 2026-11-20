import 'package:admin_app/core/db/attendance_store.dart';

/// In-memory store mirroring the contract the libsql implementation follows.
/// Used by the pipeline and sync tests; no plugins or native code involved.
class FakeStore implements AttendanceStore {
  final List<AttendanceRecord> rows = [];
  final Map<String, Student> students = {};
  int _nextId = 1;

  @override
  Future<void> open() async {}

  @override
  Future<void> close() async {}

  @override
  Future<void> addStudent(Student student) async {
    students[student.id] = student;
  }

  @override
  Future<List<Student>> allStudents() async => students.values.toList();

  @override
  Future<Student?> studentById(String id) async => students[id];

  @override
  Future<void> deleteStudent(String id) async => students.remove(id);

  @override
  Future<AttendanceRecord> insertAttendance(AttendanceRecord record) async {
    final stored = AttendanceRecord(
      id: _nextId++,
      studentId: record.studentId,
      studentName: record.studentName,
      status: record.status,
      markedAt: record.markedAt,
      deviceId: record.deviceId,
      synced: record.synced,
    );
    rows.add(stored);
    return stored;
  }

  @override
  Future<void> markSynced(List<int> ids) async {
    for (var i = 0; i < rows.length; i++) {
      if (ids.contains(rows[i].id)) {
        rows[i] = _withSynced(rows[i], true);
      }
    }
  }

  @override
  Future<List<AttendanceRecord>> pendingSync({int limit = 50}) async =>
      rows.where((r) => !r.synced).take(limit).toList();

  @override
  Future<int> pendingSyncCount() async =>
      rows.where((r) => !r.synced).length;

  @override
  Future<List<AttendanceRecord>> attendanceForDay(DateTime utcDay) async {
    final key = _day(utcDay);
    return rows.where((r) => _day(r.markedAt) == key).toList()
      ..sort((a, b) => b.markedAt.compareTo(a.markedAt));
  }

  @override
  Future<bool> hasMarkOnDay(
    String studentId,
    String status,
    DateTime utcDay,
  ) async {
    final key = _day(utcDay);
    return rows.any(
      (r) =>
          r.studentId == studentId && r.status == status && _day(r.markedAt) == key,
    );
  }

  AttendanceRecord _withSynced(AttendanceRecord record, bool synced) =>
      AttendanceRecord(
        id: record.id,
        studentId: record.studentId,
        studentName: record.studentName,
        status: record.status,
        markedAt: record.markedAt,
        deviceId: record.deviceId,
        synced: synced,
      );

  String _day(DateTime t) => t.toUtc().toIso8601String().substring(0, 10);
}
