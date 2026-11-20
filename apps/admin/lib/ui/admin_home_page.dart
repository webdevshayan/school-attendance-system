import 'dart:async';
import 'dart:io';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:path_provider/path_provider.dart';

import '../core/attendance/attendance_pipeline.dart';
import '../core/config.dart';
import '../core/db/attendance_store.dart';
import '../core/db/libsql_store.dart';
import '../core/recognition/face_service.dart';
import '../core/sync/sync_service.dart';
import 'enroll_page.dart';

enum BootStage { booting, ready, degraded }

/// Door-side home: camera preview, live recognition loop, today's marks and
/// sync state. Every plugin is optional at runtime - if the camera, the face
/// engine or the local database is unavailable, the UI degrades with a clear
/// message instead of crashing (important for half-set-up devices and for
/// CI widget tests, where no plugins exist).
class AdminHomePage extends StatefulWidget {
  const AdminHomePage({super.key});

  @override
  State<AdminHomePage> createState() => _AdminHomePageState();
}

class _AdminHomePageState extends State<AdminHomePage> {
  final FaceService _face = FaceService(threshold: AdminConfig.matchThreshold);

  AttendanceStore? _store;
  AttendancePipeline? _pipeline;
  SyncService? _sync;
  CameraController? _camera;
  Timer? _captureTimer;
  Timer? _syncTimer;

  bool _capturing = false;
  String _mode = 'in'; // what a recognised face gets marked as: 'in' | 'out'
  BootStage _stage = BootStage.booting;
  String _bootMessage = 'Starting the door...';
  String _status = 'Watching the door...';
  String _lastEvent = 'No attendance marked yet';
  int _pendingSync = 0;
  List<Student> _students = const [];
  List<AttendanceRecord> _today = const [];

  @override
  void initState() {
    super.initState();
    _bootstrap();
  }

  @override
  void dispose() {
    _captureTimer?.cancel();
    _syncTimer?.cancel();
    _camera?.dispose();
    super.dispose();
  }

  void _fail(String message) {
    if (!mounted) return;
    setState(() {
      _stage = BootStage.degraded;
      _bootMessage = message;
    });
  }

  Future<void> _bootstrap() async {
    setState(() {
      _stage = BootStage.booting;
      _bootMessage = 'Starting the door...';
    });
    try {
      await _face.init();
    } catch (error) {
      _fail('Face engine unavailable: $error');
      return;
    }
    try {
      final dir = await getApplicationCacheDirectory();
      final store = LibsqlAttendanceStore(path: '${dir.path}/attendance.db');
      await store.open();
      _store = store;
      _pipeline =
          AttendancePipeline(store: store, deviceId: AdminConfig.deviceId);
      _sync = SyncService(store: store);
    } catch (error) {
      _fail('Local database unavailable: $error');
      return;
    }
    try {
      final cameras = await availableCameras();
      final back = cameras.firstWhere(
        (camera) => camera.lensDirection == CameraLensDirection.back,
        orElse: () => cameras.first,
      );
      final controller = CameraController(back, ResolutionPreset.medium);
      await controller.initialize();
      _camera = controller;
    } catch (error) {
      _fail('Camera unavailable: $error');
      return;
    }
    await _refreshStats();
    if (!mounted) return;
    setState(() {
      _stage = BootStage.ready;
      _status = 'Watching the door...';
    });
    _captureTimer = Timer.periodic(
      AdminConfig.captureInterval,
      (_) => _captureTick(),
    );
    _syncTimer = Timer.periodic(AdminConfig.syncInterval, (_) => _syncTick());
  }

  /// One door-capture cycle: photo -> on-device verify -> attendance row.
  Future<void> _captureTick() async {
    final camera = _camera;
    final pipeline = _pipeline;
    final store = _store;
    if (_capturing ||
        !mounted ||
        _stage != BootStage.ready ||
        camera == null ||
        pipeline == null ||
        store == null ||
        !camera.value.isInitialized) {
      return;
    }
    _capturing = true;
    String? shotPath;
    try {
      shotPath = (await camera.takePicture()).path;
      final matchId = await _face.verify(shotPath);
      if (matchId == null) {
        setState(() => _status = 'No enrolled match in frame');
        return;
      }
      final student = await store.studentById(matchId);
      final name = student?.name ?? matchId;
      final result = await pipeline.recordMatch(
        studentId: matchId,
        studentName: name,
        status: _mode,
      );
      if (result.decision == PipelineDecision.recorded) {
        final record = result.record!;
        _lastEvent = '$name marked ${record.status.toUpperCase()} at '
            '${_clock(record.markedAt.toLocal())}';
        unawaited(_syncTick());
      } else if (result.decision == PipelineDecision.alreadyMarkedToday) {
        _lastEvent = '$name already marked "$_mode" today';
      }
      setState(() => _status = 'Recognised: $name');
      await _refreshStats();
    } catch (error) {
      // A failed frame must never take the door loop down.
      setState(() => _status = 'Capture error (kept going): $error');
    } finally {
      if (shotPath != null) {
        try {
          await File(shotPath).delete();
        } catch (_) {}
      }
      _capturing = false;
      if (mounted) setState(() {});
    }
  }

  Future<void> _syncTick() async {
    final sync = _sync;
    if (sync == null || !mounted) return;
    final pushed = await sync.syncPending();
    await _refreshStats();
    if (mounted && pushed > 0) {
      setState(() => _status = 'Synced $pushed record(s) to the server');
    }
  }

  Future<void> _refreshStats() async {
    final store = _store;
    if (store == null) return;
    // Run the three reads concurrently - each hits the local database.
    final results = await Future.wait(<Future<Object?>>[
      store.allStudents(),
      store.attendanceForDay(DateTime.now().toUtc()),
      store.pendingSyncCount(),
    ]);
    if (!mounted) return;
    setState(() {
      _students = results[0]! as List<Student>;
      _today = results[1]! as List<AttendanceRecord>;
      _pendingSync = results[2]! as int;
    });
  }

  String _clock(DateTime time) =>
      '${time.hour.toString().padLeft(2, '0')}:'
      '${time.minute.toString().padLeft(2, '0')}';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('School Attendance - Admin')),
      floatingActionButton: _stage == BootStage.ready
          ? FloatingActionButton.extended(
              icon: const Icon(Icons.person_add),
              label: const Text('Enroll'),
              onPressed: _openEnroll,
            )
          : null,
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
        return _readyView();
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
          'Running degraded - capture disabled',
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

  Widget _readyView() {
    final camera = _camera;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        if (!AdminConfig.isServerConfigured)
          Card(
            color: Colors.orange.shade50,
            child: const Padding(
              padding: EdgeInsets.all(12),
              child: Row(
                children: [
                  Icon(Icons.cloud_off, size: 18),
                  SizedBox(width: 8),
                  Expanded(
                    child: Text(
                      'Server URL not configured yet - attendance is saved '
                      'offline and syncs once the real domain is set (Phase 4).',
                      style: TextStyle(fontSize: 12),
                    ),
                  ),
                ],
              ),
            ),
          ),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Text('Door mode',
                        style: Theme.of(context).textTheme.titleSmall),
                    const SizedBox(width: 16),
                    Expanded(
                      child: SegmentedButton<String>(
                        segments: const [
                          ButtonSegment(
                            value: 'in',
                            label: Text('Check IN'),
                            icon: Icon(Icons.login),
                          ),
                          ButtonSegment(
                            value: 'out',
                            label: Text('Check OUT'),
                            icon: Icon(Icons.logout),
                          ),
                        ],
                        selected: {_mode},
                        onSelectionChanged: (selection) =>
                            setState(() => _mode = selection.first),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Text(_status),
                Text(_lastEvent),
              ],
            ),
          ),
        ),
        const SizedBox(height: 12),
        AspectRatio(
          aspectRatio: 3 / 4,
          child: ClipRRect(
            borderRadius: BorderRadius.circular(12),
            child: (camera != null && camera.value.isInitialized)
                ? CameraPreview(camera)
                : const ColoredBox(
                    color: Colors.black12,
                    child: Center(child: Text('Camera preview unavailable')),
                  ),
          ),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            _statChip(Icons.school, '${_students.length} enrolled'),
            _statChip(Icons.today, '${_today.length} marks today'),
            _statChip(
              _pendingSync == 0 ? Icons.cloud_done : Icons.cloud_upload,
              _pendingSync == 0 ? 'All synced' : '$_pendingSync pending sync',
            ),
          ],
        ),
        const SizedBox(height: 12),
        Text('Today', style: Theme.of(context).textTheme.titleMedium),
        ..._today.map(
          (record) => ListTile(
            leading: Icon(
              record.status == 'in' ? Icons.login : Icons.logout,
              color: record.status == 'in' ? Colors.green : Colors.red,
            ),
            title: Text(record.studentName),
            subtitle: Text(
              '${record.status.toUpperCase()} at ${_clock(record.markedAt.toLocal())}',
            ),
            trailing: record.synced
                ? const Icon(Icons.cloud_done, size: 18)
                : const Icon(Icons.cloud_upload, size: 18),
          ),
        ),
        if (_today.isEmpty)
          const Padding(
            padding: EdgeInsets.all(8),
            child: Text('No marks yet today.'),
          ),
        TextButton.icon(
          icon: const Icon(Icons.sync),
          label: const Text('Sync now'),
          onPressed: () => unawaited(_syncTick()),
        ),
      ],
    );
  }

  Widget _statChip(IconData icon, String label) {
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

  Future<void> _openEnroll() async {
    final store = _store;
    if (store == null) return;
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => EnrollPage(
          store: store,
          face: _face,
          camera: _camera,
          onChanged: _refreshStats,
        ),
      ),
    );
    await _refreshStats();
  }
}
