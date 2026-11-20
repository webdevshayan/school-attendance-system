/// A student enrolled at the school.
class Student {
  const Student({
    required this.id,
    required this.name,
    required this.createdAt,
  });

  final String id;
  final String name;
  final DateTime createdAt;
}

/// One attendance mark captured at the door.
class AttendanceRecord {
  const AttendanceRecord({
    required this.id,
    required this.studentId,
    required this.studentName,
    required this.status,
    required this.markedAt,
    required this.deviceId,
    required this.synced,
  });

  /// Row id (assigned by the store on insert; 0 before insert).
  final int id;

  final String studentId;
  final String studentName;

  /// 'in' or 'out'.
  final String status;

  /// UTC instant of the mark.
  final DateTime markedAt;

  final String deviceId;

  /// True once /api/sync has acknowledged this row.
  final bool synced;
}

/// Local persistence for students and the offline attendance queue.
/// Implementations must work fully offline; syncing is a separate concern
/// handled by [../sync/sync_service.dart].
abstract class AttendanceStore {
  Future<void> open();
  Future<void> close();

  Future<void> addStudent(Student student);
  Future<List<Student>> allStudents();
  Future<Student?> studentById(String id);
  Future<void> deleteStudent(String id);

  /// Inserts a mark and returns it with its assigned row id.
  Future<AttendanceRecord> insertAttendance(AttendanceRecord record);

  /// Flags rows as acknowledged by the server.
  Future<void> markSynced(List<int> ids);

  /// Rows not yet acknowledged, oldest first.
  Future<List<AttendanceRecord>> pendingSync({int limit = 50});

  /// How many rows are waiting to sync.
  Future<int> pendingSyncCount();

  /// Attendance marks for the given UTC calendar day, newest first.
  Future<List<AttendanceRecord>> attendanceForDay(DateTime utcDay);

  /// Whether [studentId] already has a [status] mark on [utcDay].
  Future<bool> hasMarkOnDay(String studentId, String status, DateTime utcDay);
}
