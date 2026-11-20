import 'dart:async';

import 'package:flutter/foundation.dart' show kDebugMode;
import 'package:flutter/material.dart';

import '../core/config.dart';
import '../core/db/local_store.dart';
import '../core/models/attendance_event.dart';
import '../core/pairing/event_poller.dart';
import '../core/pairing/pairing_preferences.dart';
import '../core/pairing/pairing_service.dart';
import '../core/push/push_handler.dart';
import 'pairing_sheet.dart';

enum BootStage { booting, ready, degraded }

/// Parent dashboard: today's status hero, weekly summary and the event
/// timeline, fed by the local cache and refreshed live as pushes arrive.
/// Phase 5 adds the pairing flow: an unpaired install shows a pairing
/// banner; a paired one polls the server's events feed into the same
/// PushHandler the live push channel will use later.
/// Storage is optional at runtime - when the plugin layer is missing (fresh
/// CI widget test, half-set-up device) the page degrades with a Retry
/// button instead of crashing.
class DashboardPage extends StatefulWidget {
  const DashboardPage({super.key});

  @override
  State<DashboardPage> createState() => _DashboardPageState();
}

class _DashboardPageState extends State<DashboardPage> {
  BootStage _stage = BootStage.booting;
  String _bootMessage = 'Loading dashboard...';
  LocalAttendanceStore? _store;
  PushHandler? _push;
  StreamSubscription<AttendanceEvent>? _pushSub;
  PairingService? _pairing;
  EventPoller? _poller;
  PairingPreferences? _pollerPrefs;
  String? _studentId;
  List<AttendanceEvent> _today = const [];
  bool _checkedInToday = false;
  int _presentDays = 0;

  @override
  void initState() {
    super.initState();
    _bootstrap();
  }

  @override
  void dispose() {
    _pushSub?.cancel();
    _poller?.stop();
    _push?.dispose();
    _store?.close();
    super.dispose();
  }

  Future<void> _bootstrap() async {
    setState(() {
      _stage = BootStage.booting;
      _bootMessage = 'Loading dashboard...';
    });
    try {
      final store = await LocalAttendanceStore.openDefault();
      _store = store;
      final push = PushHandler(store: store);
      _push = push;
      _pushSub = push.events.listen((_) => _refresh());
      // Pairing is optional infrastructure: a missing plugin layer degrades
      // to the debug fallback instead of blocking the whole dashboard.
      try {
        final prefs = SqflitePairingPreferences();
        final service = PairingService(prefs: prefs);
        _pairing = service;
        _pollerPrefs = prefs;
        _studentId = await service.pairedStudentId();
        _startPoller();
      } catch (_) {
        _studentId = null;
      }
    } catch (error) {
      _fail('Local storage unavailable: $error');
      return;
    }
    await _refresh();
    if (!mounted) return;
    setState(() => _stage = BootStage.ready);
  }

  void _startPoller() {
    final push = _push;
    final service = _pairing;
    final prefs = _pollerPrefs;
    if (push == null || service == null || prefs == null) return;
    _poller?.stop();
    final poller = EventPoller(
      pairing: service,
      prefs: prefs,
      push: push,
      onRevoked: () => _reloadPairing(),
    );
    _poller = poller;
    poller.start();
  }

  /// Re-reads the pairing state (after pairing or a server-side revocation).
  Future<void> _reloadPairing() async {
    final service = _pairing;
    if (service == null) return;
    _studentId = await service.pairedStudentId();
    if (!mounted) return;
    if (_studentId != null) {
      _startPoller();
    } else {
      _poller?.stop();
    }
    setState(() {});
  }

  void _fail(String message) {
    if (!mounted) return;
    setState(() {
      _stage = BootStage.degraded;
      _bootMessage = message;
    });
  }

  Future<void> _refresh() async {
    final store = _store;
    if (store == null) return;
    final studentId = _studentId ?? ChildConfig.childId;
    final now = DateTime.now().toUtc();
    final today = await store.eventsForDay(now);
    final checkedIn = await store.hasStatusOnDay(studentId, 'in', now);
    final days = await store.presentDaysLastWeek(studentId, now);
    if (!mounted) return;
    setState(() {
      _today = today;
      _checkedInToday = checkedIn;
      _presentDays = days;
    });
  }

  Future<void> _openPairingSheet() async {
    final service = _pairing;
    if (service == null) return;
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (_) => PairingSheet(
        service: service,
        onPaired: (_) => _reloadPairing(),
      ),
    );
  }

  /// Debug hook: feeds a sample push through the real pipeline so the
  /// dashboard can be exercised before the install is paired.
  Future<void> _simulatePush() async {
    final push = _push;
    if (push == null) return;
    await push.handle(<String, dynamic>{
      'student_id': _studentId ?? ChildConfig.childId,
      'student_name': ChildConfig.childName,
      'status': _checkedInToday ? 'out' : 'in',
      'marked_at': DateTime.now().toUtc().toIso8601String(),
      'device_id': 'door-1',
    });
  }

  String _clock(DateTime time) =>
      '${time.hour.toString().padLeft(2, '0')}:'
      '${time.minute.toString().padLeft(2, '0')}';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('School Attendance - Parent')),
      body: _body(),
    );
  }

  Widget _body() {
    switch (_stage) {
      case BootStage.booting:
        return _center([
          const CircularProgressIndicator(),
          const SizedBox(height: 16),
          Text(_bootMessage),
        ]);
      case BootStage.degraded:
        return _degradedView();
      case BootStage.ready:
        return _dashboard();
    }
  }

  Widget _center(List<Widget> children) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: children,
        ),
      );

  Widget _degradedView() {
    return ListView(
      padding: const EdgeInsets.all(24),
      children: [
        Icon(Icons.report_problem, size: 64, color: Colors.orange.shade700),
        const SizedBox(height: 16),
        const Text(
          'Dashboard unavailable',
          textAlign: TextAlign.center,
          style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
        ),
        const SizedBox(height: 8),
        Text(_bootMessage, textAlign: TextAlign.center),
        const SizedBox(height: 24),
        Center(
          child: FilledButton.icon(
            icon: const Icon(Icons.refresh),
            label: const Text('Retry'),
            onPressed: _bootstrap,
          ),
        ),
      ],
    );
  }

  Widget _dashboard() {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        if (_studentId == null) _pairingBanner(),
        if (_studentId != null && _poller?.lastError != null) _feedErrorBanner(),
        _heroCard(),
        const SizedBox(height: 12),
        _chip(Icons.event_available, '$_presentDays/7 days this week'),
        const SizedBox(height: 12),
        Text('Today', style: Theme.of(context).textTheme.titleMedium),
        ..._today.map(
          (event) => ListTile(
            leading: Icon(
              event.status == 'in' ? Icons.login : Icons.logout,
              color: event.status == 'in' ? Colors.green : Colors.red,
            ),
            title: Text(event.studentName),
            subtitle: Text(
              '${event.status.toUpperCase()} at ${_clock(event.markedAt.toLocal())}',
            ),
            trailing: const Icon(Icons.notifications_active, size: 18),
          ),
        ),
        if (_today.isEmpty)
          const Padding(
            padding: EdgeInsets.all(8),
            child: Text(
              'No attendance yet today. Pushes from the school door will '
              'appear here the moment they arrive.',
            ),
          ),
        // Debug-only: feeds a sample payload through the real pipeline so
        // the dashboard is exercisable before the live push channel exists.
        if (kDebugMode)
          TextButton.icon(
            icon: const Icon(Icons.bug_report),
            label: const Text('Simulate a push (debug)'),
            onPressed: _simulatePush,
          ),
      ],
    );
  }

  /// Banner shown while this install follows the debug fallback student.
  Widget _pairingBanner() {
    return Card(
      color: Colors.blue.shade50,
      margin: const EdgeInsets.only(bottom: 12),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.link_off, color: Colors.blue.shade800),
                const SizedBox(width: 8),
                const Expanded(
                  child: Text(
                    'This device isn\'t paired to a child yet',
                    style: TextStyle(fontWeight: FontWeight.bold),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            const Text(
              'Enter the pairing code the school gave you to see your '
              'child\'s live attendance. Until then the dashboard shows '
              'demo data only.',
            ),
            const SizedBox(height: 12),
            FilledButton.icon(
              icon: const Icon(Icons.link),
              label: const Text('Pair with a code'),
              onPressed: _openPairingSheet,
            ),
          ],
        ),
      ),
    );
  }

  Widget _feedErrorBanner() {
    return Card(
      color: Colors.orange.shade50,
      margin: const EdgeInsets.only(bottom: 12),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          children: [
            Icon(Icons.wifi_off, color: Colors.orange.shade800, size: 18),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                'Live updates paused: ${_poller?.lastError}',
                style: const TextStyle(fontSize: 13),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _heroCard() {
    final inSchool = _checkedInToday;
    return Card(
      clipBehavior: Clip.antiAlias,
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.all(20),
        decoration: BoxDecoration(
          gradient: LinearGradient(
            colors: inSchool
                ? [Colors.green.shade600, Colors.teal.shade500]
                : [Colors.orange.shade600, Colors.deepOrange.shade400],
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              ChildConfig.childName,
              style: const TextStyle(
                color: Colors.white,
                fontSize: 14,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(height: 8),
            Row(
              children: [
                Icon(
                  inSchool ? Icons.check_circle : Icons.schedule,
                  color: Colors.white,
                  size: 40,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    inSchool
                        ? 'Checked in at school today'
                        : 'Not checked in yet today',
                    style: const TextStyle(
                      color: Colors.white,
                      fontSize: 18,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _chip(IconData icon, String label) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 16),
          const SizedBox(width: 6),
          Text(label),
        ],
      ),
    );
  }
}
