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

  /// Who the enrollment attaches to after the identify pass discovered the
  /// captured face is already enrolled. `adopt` = replace the matched
  /// student's sample: the enrollment keeps THEIR id (the door keeps
  /// recognizing the same human) but takes the freshly typed name, so name
  /// corrections ride along for free. `!adopt` = the operator insists this
  /// is a different student and the capture enrolls under the typed id.
  ///
  /// Ids are internal machine keys - the flow is name-first and the face
  /// itself is the lookup key, so a school with hundreds of students never
  /// needs to distribute or type ids.
  static ({String id, String name}) resolveDuplicate({
    required Student matched,
    required String typedId,
    required String typedName,
    required bool adopt,
  }) =>
      adopt
          ? (id: matched.id, name: typedName)
          : (id: typedId, name: typedName);
}
