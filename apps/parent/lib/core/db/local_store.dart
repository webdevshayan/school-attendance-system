import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:sqflite/sqflite.dart';

import '../models/attendance_event.dart';
import 'event_store.dart';

/// Local cache of attendance events received by this device. Works fully
/// offline; Phase 4 populates it from the server's push fan-out while the
/// dashboard reads it directly.
class LocalAttendanceStore implements EventStore {
  LocalAttendanceStore({required String path}) : _path = path;

  final String _path;
  Database? _db;

  Database get _database {
    final db = _db;
    if (db == null) {
      throw StateError('LocalAttendanceStore.open() must be called first');
    }
    return db;
  }

  /// Opens the store at the app documents directory.
  static Future<LocalAttendanceStore> openDefault() async {
    final dir = await getApplicationDocumentsDirectory();
    final store = LocalAttendanceStore(
      path: p.join(dir.path, 'parent_attendance.db'),
    );
    await store.open();
    return store;
  }

  Future<void> open() async {
    final db = await openDatabase(
      _path,
      version: 1,
      onCreate: (db, version) async {
        await db.execute('''
          create table events (
            id integer primary key autoincrement,
            student_id text not null,
            student_name text not null,
            status text not null,
            marked_at text not null,
            device_id text not null,
            unique(student_id, status, marked_at)
          )
        ''');
      },
    );
    _db = db;
  }

  Future<void> close() async {
    await _db?.close();
    _db = null;
  }

  @override
  Future<void> addEvent(AttendanceEvent event) async {
    await _database.insert(
      'events',
      <String, Object?>{
        'student_id': event.studentId,
        'student_name': event.studentName,
        'status': event.status,
        'marked_at': event.markedAt.toUtc().toIso8601String(),
        'device_id': event.deviceId,
      },
      // Duplicate pushes (same student/status/instant) are ignored instead
      // of duplicating rows on the timeline.
      conflictAlgorithm: ConflictAlgorithm.ignore,
    );
  }

  /// Events for the given UTC calendar day, newest first.
  Future<List<AttendanceEvent>> eventsForDay(DateTime utcDay) async {
    final key = utcDay.toUtc().toIso8601String().substring(0, 10);
    final rows = await _database.query(
      'events',
      where: 'substr(marked_at, 1, 10) = ?',
      whereArgs: <Object?>[key],
      orderBy: 'marked_at desc',
    );
    return rows.map(_fromRow).toList();
  }

  /// Whether [studentId] has a [status] event recorded on [utcDay].
  Future<bool> hasStatusOnDay(
    String studentId,
    String status,
    DateTime utcDay,
  ) async {
    final key = utcDay.toUtc().toIso8601String().substring(0, 10);
    final rows = await _database.query(
      'events',
      where:
          'student_id = ? and status = ? and substr(marked_at, 1, 10) = ?',
      whereArgs: <Object?>[studentId, status, key],
      limit: 1,
    );
    return rows.isNotEmpty;
  }

  /// Distinct days with an 'in' mark in the last 7 UTC days.
  Future<int> presentDaysLastWeek(String studentId, DateTime nowUtc) async {
    final from = nowUtc.toUtc().subtract(const Duration(days: 7));
    final rows = await _database.rawQuery('''
      select count(distinct substr(marked_at, 1, 10)) as n
      from events
      where student_id = ? and status = 'in' and marked_at >= ?
    ''', <Object?>[studentId, from.toIso8601String()]);
    return (rows.first['n'] as int?) ?? 0;
  }

  AttendanceEvent _fromRow(Map<String, Object?> row) => AttendanceEvent(
        studentId: row['student_id'] as String,
        studentName: row['student_name'] as String,
        status: row['status'] as String,
        markedAt: DateTime.parse(row['marked_at'] as String),
        deviceId: row['device_id'] as String? ?? '',
      );
}
