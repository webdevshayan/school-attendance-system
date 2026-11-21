import '../db/attendance_store.dart';

/// Pure search helpers over the student roster, unit-tested without any
/// plugin. The enroll page applies them to the in-memory student list.
class RosterSearch {
  const RosterSearch._();

  /// Case-insensitive substring filter over student names. A blank query
  /// (empty or whitespace-only) returns every student in the same order.
  ///
  /// Ids are deliberately NOT searchable: they are internal machine keys
  /// (name-first flow) and must never become something the operator types
  /// to find a student.
  static List<Student> filter(List<Student> students, String query) {
    final needle = query.trim().toLowerCase();
    if (needle.isEmpty) return List.of(students);
    return students
        .where((student) => student.name.toLowerCase().contains(needle))
        .toList();
  }
}
