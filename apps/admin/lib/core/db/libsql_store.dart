import 'package:libsql_dart/libsql_dart.dart';

import 'attendance_store.dart';

/// Production [AttendanceStore] backed by a local libSQL (SQLite) file via
/// libsql_dart. Works fully offline; Phase 4 can move the same client to an
/// embedded replica pointed at Turso without changing any callers.
class LibsqlAttendanceStore implements AttendanceStore {
  LibsqlAttendanceStore({required String path}) : _path = path;

  final String _path;
  LibsqlClient? _client;

  LibsqlClient get _db {
    final client = _client;
    if (client == null) {
      throw StateError('LibsqlAttendanceStore.open() must be called first');
    }
    return client;
  }

  @override
  Future<void> open() async {
    final client = LibsqlClient.local(_path);
    await client.connect();
    await client.execute('''
      create table if not exists students (
        id text primary key,
        name text not null,
        created_at text not null
      )
    ''');
    await client.execute('''
      create table if not exists attendance (
        id integer primary key autoincrement,
        student_id text not null,
        student_name text not null,
        status text not null,
        marked_at text not null,
        device_id text not null,
        synced integer not null default 0
      )
    ''');
    await client.execute(
      'create index if not exists idx_attendance_synced on attendance (synced)',
    );
    _client = client;
  }

  @override
  Future<void> close() async {
    await _client?.dispose();
    _client = null;
  }

  // -- students -----------------------------------------------------------

  @override
  Future<void> addStudent(Student student) => _db.execute(
        'insert or replace into students (id, name, created_at) values (?, ?, ?)',
        positional: [
          student.id,
          student.name,
          student.createdAt.toUtc().toIso8601String(),
        ],
      );

  @override
  Future<List<Student>> allStudents() async {
    final rows = await _db.query('select * from students order by name');
    return rows.map(_studentFromRow).toList();
  }

  @override
  Future<Student?> studentById(String id) async {
    final rows = await _db.query(
      'select * from students where id = ?',
      positional: [id],
    );
    return rows.isEmpty ? null : _studentFromRow(rows.first);
  }

  @override
  Future<void> deleteStudent(String id) => _db.execute(
        'delete from students where id = ?',
        positional: [id],
      );

  Student _studentFromRow(Map<String, dynamic> row) => Student(
        id: row['id'] as String,
        name: row['name'] as String,
        createdAt: DateTime.parse(row['created_at'] as String),
      );

  // -- attendance ---------------------------------------------------------

  @override
  Future<AttendanceRecord> insertAttendance(AttendanceRecord record) async {
    await _db.execute(
      '''insert into attendance
         (student_id, student_name, status, marked_at, device_id, synced)
         values (?, ?, ?, ?, ?, ?)''',
      positional: [
        record.studentId,
        record.studentName,
        record.status,
        record.markedAt.toUtc().toIso8601String(),
        record.deviceId,
        record.synced ? 1 : 0,
      ],
    );
    final rows = await _db.query('select last_insert_rowid() as id');
    return AttendanceRecord(
      id: rows.first['id'] as int,
      studentId: record.studentId,
      studentName: record.studentName,
      status: record.status,
      markedAt: record.markedAt,
      deviceId: record.deviceId,
      synced: record.synced,
    );
  }

  @override
  Future<void> markSynced(List<int> ids) async {
    for (final id in ids) {
      await _db.execute(
        'update attendance set synced = 1 where id = ?',
        positional: [id],
      );
    }
  }

  @override
  Future<List<AttendanceRecord>> pendingSync({int limit = 50}) async {
    final rows = await _db.query(
      'select * from attendance where synced = 0 order by id asc limit ?',
      positional: [limit],
    );
    return rows.map(_recordFromRow).toList();
  }

  @override
  Future<int> pendingSyncCount() async {
    final rows = await _db.query(
      'select count(*) as n from attendance where synced = 0',
    );
    return rows.first['n'] as int;
  }

  @override
  Future<List<AttendanceRecord>> attendanceForDay(DateTime utcDay) async {
    final rows = await _db.query(
      '''select * from attendance
         where substr(marked_at, 1, 10) = ?
         order by marked_at desc''',
      positional: [_dayKey(utcDay)],
    );
    return rows.map(_recordFromRow).toList();
  }

  @override
  Future<bool> hasMarkOnDay(
    String studentId,
    String status,
    DateTime utcDay,
  ) async {
    final rows = await _db.query(
      '''select count(*) as n from attendance
         where student_id = ? and status = ? and substr(marked_at, 1, 10) = ?''',
      positional: [studentId, status, _dayKey(utcDay)],
    );
    return (rows.first['n'] as int) > 0;
  }

  @override
  Future<List<AttendanceRecord>> attendanceBetween(
    DateTime startUtc,
    DateTime endUtc,
  ) async {
    final rows = await _db.query(
      '''select * from attendance
         where marked_at >= ? and marked_at < ?
         order by marked_at desc''',
      positional: [_boundsKey(startUtc), _boundsKey(endUtc)],
    );
    return rows.map(_recordFromRow).toList();
  }

  AttendanceRecord _recordFromRow(Map<String, dynamic> row) => AttendanceRecord(
        id: row['id'] as int,
        studentId: row['student_id'] as String,
        studentName: row['student_name'] as String,
        status: row['status'] as String,
        markedAt: DateTime.parse(row['marked_at'] as String),
        deviceId: row['device_id'] as String,
        synced: (row['synced'] as int) != 0,
      );

  /// UTC calendar-day key ('YYYY-MM-DD') used for day-level queries.
  String _dayKey(DateTime day) =>
      day.toUtc().toIso8601String().substring(0, 10);

  /// Window-bound key for text comparison against stored timestamps. Stored
  /// values may or may not carry a fractional part (Dart's toIso8601String
  /// omits it on whole seconds), so bounds always carry '.000' - text
  /// ordering then behaves as a half-open window for both row styles.
  String _boundsKey(DateTime day) {
    final iso = day.toUtc().toIso8601String();
    return iso.contains('.') ? iso : '${iso.substring(0, 19)}.000Z';
  }
}
