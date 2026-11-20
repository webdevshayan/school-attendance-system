import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import '../config.dart';
import '../push/push_handler.dart';
import 'pairing_preferences.dart';
import 'pairing_service.dart';

/// Polls the server's events feed for the paired student and feeds every
/// payload into the [PushHandler] - the exact seam a live push channel will
/// use, so switching to Nitroping later changes nothing downstream.
///
/// Delivery is cursor based: each poll resumes from the last delivered row
/// id, so events are never re-broadcast after an app restart (the parent
/// store also de-duplicates, mirroring the admin app's lost-ack safety).
class EventPoller {
  EventPoller({
    required PairingService pairing,
    required PushHandler push,
    http.Client? client,
    this.baseUrl = ChildConfig.serverBaseUrl,
    this.interval = ChildConfig.pollInterval,
    this.onRevoked,
  })  : _pairing = pairing,
        _push = push,
        _client = client ?? http.Client();

  final PairingService _pairing;
  final PushHandler _push;
  final http.Client _client;
  final String baseUrl;
  final Duration interval;

  /// Called when the server revokes this install's binding (401): the
  /// dashboard listens to flip back to the pairing screen.
  final void Function()? onRevoked;

  Timer? _timer;
  bool _inFlight = false;

  /// Set when a poll fails (offline, server error, revoked binding); the
  /// dashboard surfaces it as a banner.
  String? lastError;

  /// Starts the periodic poll with an immediate first tick. Safe to call
  /// again - it restarts the timer rather than stacking two.
  void start() {
    stop();
    _timer = Timer.periodic(interval, (_) => pollOnce());
    unawaited(pollOnce());
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
  }

  /// One poll cycle: skip while unpaired or while the previous request is
  /// still in flight; deliver accepted payloads; persist the cursor.
  Future<void> pollOnce() async {
    if (_inFlight) return;
    final token = await _pairing.token();
    final studentId = await _pairing.pairedStudentId();
    if (token == null || studentId == null) return;
    _inFlight = true;
    try {
      final rawCursor = await _pairing.read(PairingKeys.cursor);
      final since = int.tryParse(rawCursor ?? '') ?? 0;
      final response = await _client.get(
        Uri.parse('$baseUrl/api/events?since=$since'),
        headers: {'Authorization': 'Bearer $token'},
      );
      if (response.statusCode == 401) {
        // Binding revoked server-side: forget it so the UI returns to the
        // pairing screen instead of looping on 401s.
        lastError = 'pairing was revoked; pair this device again';
        await _pairing.unpair();
        onRevoked?.call();
        return;
      }
      if (response.statusCode < 200 || response.statusCode >= 300) {
        lastError = 'events feed returned ${response.statusCode}';
        return;
      }
      final decoded = jsonDecode(response.body);
      if (decoded is! Map<String, dynamic>) {
        lastError = 'events feed sent an unexpected payload';
        return;
      }
      final events = decoded['events'];
      final cursor = decoded['cursor'];
      if (events is List) {
        for (final event in events) {
          if (event is Map) {
            await _push.handle(Map<String, dynamic>.from(event));
          }
        }
      }
      if (cursor is int && cursor >= 0) {
        await _pairing.write(PairingKeys.cursor, '$cursor');
      }
      lastError = null;
    } catch (_) {
      // Offline or transport error: keep the cursor, retry next tick.
      lastError = 'events feed unreachable';
    } finally {
      _inFlight = false;
    }
  }
}
