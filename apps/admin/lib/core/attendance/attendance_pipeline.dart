import '../config.dart';
import '../db/attendance_store.dart';

/// What the pipeline decided to do with a face match.
enum PipelineDecision {
  /// A new attendance row was written.
  recorded,

  /// Same student was marked too recently; ignored.
  cooldownActive,

  /// Student already has this status for today; ignored.
  alreadyMarkedToday,
}

class PipelineResult {
  const PipelineResult(this.decision, {this.record});

  final PipelineDecision decision;
  final AttendanceRecord? record;
}

/// Turns raw face matches into attendance rows, enforcing per-student
/// cooldown and one-mark-per-day rules. Pure logic, unit tested with a fake
/// store - no plugins involved.
class AttendancePipeline {
  AttendancePipeline({
    required AttendanceStore store,
    required this.deviceId,
    DateTime Function()? now,
    this.cooldown = AdminConfig.perStudentCooldown,
  })  : _store = store,
        _now = now ?? DateTime.now;

  final AttendanceStore _store;

  /// Stamped on every row this pipeline writes.
  final String deviceId;

  final DateTime Function() _now;
  final Duration cooldown;

  /// Last time we actually wrote a mark per student. In-memory and resets on
  /// restart, which is fine: the authoritative day-level rule lives in the
  /// database and survives restarts.
  final Map<String, DateTime> _lastMarkAt = {};

  Future<PipelineResult> recordMatch({
    required String studentId,
    required String studentName,
    required String status,
  }) async {
    final now = _now();

    final last = _lastMarkAt[studentId];
    if (last != null && now.difference(last) < cooldown) {
      return const PipelineResult(PipelineDecision.cooldownActive);
    }

    if (await _store.hasMarkOnDay(studentId, status, now)) {
      return const PipelineResult(PipelineDecision.alreadyMarkedToday);
    }

    final record = await _store.insertAttendance(
      AttendanceRecord(
        id: 0,
        studentId: studentId,
        studentName: studentName,
        status: status,
        markedAt: now.toUtc(),
        deviceId: deviceId,
        synced: false,
      ),
    );
    _lastMarkAt[studentId] = now;
    return PipelineResult(PipelineDecision.recorded, record: record);
  }
}
