import 'dart:async';

import '../db/event_store.dart';
import '../models/attendance_event.dart';

/// Transport-agnostic push handler: every accepted attendance payload flows
/// parse -> validate -> de-duplicate into the local store -> broadcast. The
/// production transport is the events poller (the free live channel); the
/// seam stays so any future transport can feed it unchanged.
class PushHandler {
  PushHandler({required EventStore store}) : _store = store;

  final EventStore _store;
  final _events = StreamController<AttendanceEvent>.broadcast();

  /// Emits every accepted attendance event (the dashboard listens to refresh).
  Stream<AttendanceEvent> get events => _events.stream;

  /// Handles one push payload. Returns false for malformed or invalid
  /// payloads (bad timestamp, unknown status, missing student id) without
  /// touching the store.
  Future<bool> handle(Map<String, dynamic> payload) async {
    final AttendanceEvent event;
    try {
      event = AttendanceEvent.fromJson(payload);
    } catch (_) {
      return false;
    }
    if (event.studentId.isEmpty) return false;
    if (event.status != 'in' && event.status != 'out') return false;
    await _store.addEvent(event);
    _events.add(event);
    return true;
  }

  Future<void> dispose() => _events.close();
}
