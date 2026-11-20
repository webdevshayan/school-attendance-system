import 'dart:async';

import 'package:flutter/material.dart';

import '../core/config.dart';
import '../core/db/local_store.dart';
import '../core/models/attendance_event.dart';
import '../core/push/push_handler.dart';

enum BootStage { booting, ready, degraded }

/// Parent dashboard: today's status hero, weekly summary and the event
/// timeline, fed by the local cache and refreshed live as pushes arrive.
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
    } catch (error) {
      _fail('Local storage unavailable: $error');
      return;
    }
    await _refresh();
    if (!mounted) return;
    setState(() => _stage = BootStage.ready);
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
    final now = DateTime.now().toUtc();
    final today = await store.eventsForDay(now);
    final checkedIn =
        await store.hasStatusOnDay(ChildConfig.childId, 'in', now);
    final days = await store.presentDaysLastWeek(ChildConfig.childId, now);
    if (!mounted) return;
    setState(() {
      _today = today;
      _checkedInToday = checkedIn;
      _presentDays = days;
    });
  }

  /// Debug hook: feeds a sample push through the real pipeline so the
  /// dashboard can be exercised before Phase 4 wires the live channel.
  Future<void> _simulatePush() async {
    final push = _push;
    if (push == null) return;
    await push.handle(<String, dynamic>{
      'student_id': ChildConfig.childId,
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
        const SizedBox(height: 12),
        TextButton.icon(
          icon: const Icon(Icons.bug_report),
          label: const Text('Simulate a push (debug)'),
          onPressed: _simulatePush,
        ),
      ],
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
