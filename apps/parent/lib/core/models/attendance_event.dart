/// One attendance event for a child, as delivered by a push (Phase 4 fan-out)
/// or read back from local storage. Mirrors the wire payload the admin app
/// posts to `/api/sync`, plus a display name.
class AttendanceEvent {
  const AttendanceEvent({
    required this.studentId,
    required this.studentName,
    required this.status,
    required this.markedAt,
    this.deviceId = '',
  });

  final String studentId;
  final String studentName;

  /// 'in' or 'out'.
  final String status;

  /// UTC instant of the mark.
  final DateTime markedAt;
  final String deviceId;

  factory AttendanceEvent.fromJson(Map<String, dynamic> json) {
    final studentId = json['student_id'] as String? ?? '';
    return AttendanceEvent(
      studentId: studentId,
      studentName: json['student_name'] as String? ?? studentId,
      status: json['status'] as String? ?? 'in',
      markedAt: DateTime.parse(json['marked_at'] as String),
      deviceId: json['device_id'] as String? ?? '',
    );
  }

  Map<String, dynamic> toJson() => <String, dynamic>{
        'student_id': studentId,
        'student_name': studentName,
        'status': status,
        'marked_at': markedAt.toUtc().toIso8601String(),
        'device_id': deviceId,
      };
}
