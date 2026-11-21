import '../db/attendance_store.dart';

/// Pure decision helpers for the enroll flow, unit-tested without camera or
/// face-engine plugins. The enroll page applies them around the real calls.
class EnrollGuard {
  const EnrollGuard._();

  /// The self-check: right after registering a face sample, the engine must
  /// recognize the SAME photo back as the same id. A null match (or a
  /// different id) means the sample is too poor to trust at the door, where
  /// it would quietly produce "no match" for the very student who enrolled.
  static bool sampleTrusted(String? selfMatchId, String enrolledId) =>
      selfMatchId == enrolledId;

  /// Re-enrolling an existing id silently REPLACES the stored sample (the
  /// face plugin replaces samples and the store inserts-or-replaces the
  /// row) - that deserves a confirmation instead of a surprise.
  static Student? findExisting(List<Student> students, String id) {
    for (final student in students) {
      if (student.id == id) return student;
    }
    return null;
  }
}
