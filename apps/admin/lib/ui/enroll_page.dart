import 'dart:async';
import 'dart:io';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';

import '../core/db/attendance_store.dart';
import '../core/pairing/code_service.dart';
import '../core/recognition/face_service.dart';

/// Registers a student: name (+ optional id) -> camera snap -> on-device
/// face sample + local database row. Each student row also mints a parent
/// pairing code (Phase 5) so the guardians can follow that child's
/// attendance from the parent app.
class EnrollPage extends StatefulWidget {
  const EnrollPage({
    super.key,
    required this.store,
    required this.face,
    required this.camera,
    required this.onChanged,
    this.codeService,
  });

  final AttendanceStore store;
  final FaceService face;
  final CameraController? camera;
  final Future<void> Function() onChanged;

  /// Injectable for tests; defaults to a production-configured service.
  final CodeService? codeService;

  @override
  State<EnrollPage> createState() => _EnrollPageState();
}

class _EnrollPageState extends State<EnrollPage> {
  final _nameController = TextEditingController();
  final _idController = TextEditingController();
  bool _busy = false;
  List<Student> _students = const [];

  @override
  void initState() {
    super.initState();
    _reload();
  }

  @override
  void dispose() {
    _nameController.dispose();
    _idController.dispose();
    super.dispose();
  }

  Future<void> _reload() async {
    final students = await widget.store.allStudents();
    if (!mounted) return;
    setState(() => _students = students);
  }

  Future<void> _enroll() async {
    final name = _nameController.text.trim();
    if (name.isEmpty) {
      _snack('Enter the student name first');
      return;
    }
    final camera = widget.camera;
    if (camera == null || !camera.value.isInitialized) {
      _snack('Camera is not available');
      return;
    }
    setState(() => _busy = true);
    String? shotPath;
    try {
      shotPath = (await camera.takePicture()).path;
      final id = _idController.text.trim().isEmpty
          ? 's_${DateTime.now().millisecondsSinceEpoch}'
          : _idController.text.trim();
      await widget.face.register(
        studentId: id,
        imagePath: shotPath,
        name: name,
      );
      await widget.store.addStudent(
        Student(id: id, name: name, createdAt: DateTime.now().toUtc()),
      );
      _nameController.clear();
      _idController.clear();
      await widget.onChanged();
      await _reload();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('$name enrolled as $id')),
        );
      }
    } catch (error) {
      _snack('Enrollment failed: $error');
    } finally {
      if (shotPath != null) {
        try {
          await File(shotPath).delete();
        } catch (_) {}
      }
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _remove(Student student) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('Remove ${student.name}?'),
        content: const Text(
          'Their face sample and local record will be deleted from this device.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Remove'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;
    try {
      await widget.face.forget(student.id);
    } catch (_) {}
    await widget.store.deleteStudent(student.id);
    await widget.onChanged();
    await _reload();
  }

  void _snack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  /// Mints and displays a parent-pairing code for [student]. The dialog
  /// resolves the mint future so failures surface inside it as retry hints.
  Future<void> _showPairingCode(Student student) async {
    final service = widget.codeService ?? CodeService();
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text('Pairing code - ${student.name}'),
        content: FutureBuilder<PairingCodeInfo?>(
          future: service.mint(student.id),
          builder: (context, snapshot) {
            if (snapshot.connectionState != ConnectionState.done) {
              return const SizedBox(
                height: 48,
                child: Center(child: CircularProgressIndicator()),
              );
            }
            final info = snapshot.data;
            if (info == null) {
              return const Text(
                'Could not mint a code right now. Check connectivity and '
                'that this device is registered, then try again.',
              );
            }
            return Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                SelectableText(
                  info.code,
                  style: const TextStyle(
                    fontSize: 28,
                    fontWeight: FontWeight.bold,
                    letterSpacing: 4,
                  ),
                ),
                const SizedBox(height: 8),
                Text(
                  'Give this code to ${student.name}\'s guardians. Both '
                  'parents can pair with it until '
                  '${info.expiresAt.toLocal().toString().split(' ').first}.',
                ),
              ],
            );
          },
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: const Text('Done'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Enroll students')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          TextField(
            controller: _nameController,
            decoration: const InputDecoration(
              labelText: 'Student name',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _idController,
            decoration: const InputDecoration(
              labelText: 'Student id (optional)',
              helperText: 'Left empty, an id is generated automatically',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          FilledButton.icon(
            icon: _busy
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.camera_alt),
            label: Text(_busy ? 'Capturing...' : 'Capture face & enroll'),
            onPressed: _busy ? null : _enroll,
          ),
          const SizedBox(height: 24),
          Text('Enrolled (${_students.length})',
              style: Theme.of(context).textTheme.titleMedium),
          ..._students.map(
            (student) => ListTile(
              leading: const Icon(Icons.face),
              title: Text(student.name),
              subtitle: Text(student.id),
              trailing: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  IconButton(
                    tooltip: 'Pairing code for the guardians',
                    icon: const Icon(Icons.key_outline),
                    onPressed: () => unawaited(_showPairingCode(student)),
                  ),
                  IconButton(
                    icon: const Icon(Icons.delete_outline),
                    onPressed: () => unawaited(_remove(student)),
                  ),
                ],
              ),
            ),
          ),
          if (_students.isEmpty)
            const Padding(
              padding: EdgeInsets.all(8),
              child: Text('Nobody enrolled yet. Capture a face to start.'),
            ),
        ],
      ),
    );
  }
}
