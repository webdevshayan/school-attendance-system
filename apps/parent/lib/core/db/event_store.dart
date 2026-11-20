import '../models/attendance_event.dart';

/// Minimal sink contract for accepted attendance events. Implemented by the
/// local store; fakes in tests.
abstract class EventStore {
  Future<void> addEvent(AttendanceEvent event);
}
