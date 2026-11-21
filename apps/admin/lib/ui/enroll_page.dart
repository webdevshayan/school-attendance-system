import 'dart:async';
import 'dart:io';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../core/db/attendance_store.dart';
import '../core/pairing/code_service.dart';
import '../core/recognition/enroll_errors.dart';
import '../core/recognition/enrollment_guard.dart';
import '../core/recognition/face_service.dart';
import '../core/roster/roster_search.dart';

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
  final _searchController = TextEditingController();
  bool _busy = false;
  List<Student> _students = const [];

  /// The visible roster: the search query filters the in-memory list, so
  /// a school with hundreds of students can still find anyone instantly.
  List<Student> get _filtered => RosterSearch.filter(
        _students,
        _searchController.text,
      );

  @override
  void initState() {
    super.initState();
    _reload();
  }

  @override
  void dispose() {
    _nameController.dispose();
    _idController.dispose();
    _searchController.dispose();
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
    var enrollId = _idController.text.trim().isEmpty
        ? 's_${DateTime.now().millisecondsSinceEpoch}'
        : _idController.text.trim();
    var enrollName = name;
    // Typing an already-enrolled id replaces that student's stored sample
    // silently - confirm first, before any capture (EnrollGuard).
    final existing = EnrollGuard.findExisting(_students, enrollId);
    if (existing != null) {
      final confirmed = await _confirmReplace(existing, enrollId);
      if (!confirmed) return;
    }
    setState(() => _busy = true);
    String? shotPath;
    try {
      // The face engine (ML Kit on the Java side) can fail on a bad frame
      // with a native NPE - one ugly capture must not kill enrollment.
      // Retry ONCE with a fresh capture; anything after that fails with
      // operator guidance instead of an obfuscated stack trace.
      for (var attempt = 1; attempt <= 2; attempt++) {
        shotPath = (await camera.takePicture()).path;
        // The face IS the identity key: before registering, ask the engine
        // who this capture belongs to. If it matches an already-enrolled
        // student, the operator decides (replace their sample / enroll as
        // new anyway / cancel) - that keeps a school with hundreds of
        // students self-managing without ids, cards or roll numbers.
        String? matchedId;
        try {
          matchedId = await widget.face.verify(shotPath);
        } on PlatformException catch (error) {
          // Identification is a convenience guard, not a requirement -
          // never block enrollment on it (register retries below).
          _logEngineFailure('identify', error);
        }
        if (matchedId != null && matchedId != enrollId) {
          final matched = EnrollGuard.findExisting(_students, matchedId);
          if (matched != null) {
            final adopt = await _confirmDuplicate(matched);
            if (adopt == null) return; // cancel: nothing was saved
            final plan = EnrollGuard.resolveDuplicate(
              matched: matched,
              typedId: enrollId,
              typedName: name,
              adopt: adopt,
            );
            enrollId = plan.id;
            enrollName = plan.name;
          }
        }
        // Quality gate, non-destructive (review F1): register under a
        // throwaway id first; the old sample of a replace is overwritten
        // only after the new capture proves itself.
        final tempId = 'tmp_${DateTime.now().microsecondsSinceEpoch}';
        try {
          await widget.face.register(
            studentId: tempId,
            imagePath: shotPath,
            name: enrollName,
          );
        } on PlatformException catch (error) {
          _logEngineFailure('register', error);
          await _discardTemp(tempId);
          if (attempt == 1) {
            _snack('Face engine hiccup on that frame - retaking '
                'automatically...');
            final stale = shotPath;
            shotPath = null;
            try {
              await File(stale).delete();
            } catch (_) {}
            continue;
          }
          _snack(EnrollErrors.message(error));
          return;
        }
        String? selfMatch;
        try {
          selfMatch = await widget.face.verify(shotPath);
        } on PlatformException catch (error) {
          _logEngineFailure('verify', error);
          await _discardTemp(tempId);
          _snack(EnrollErrors.message(error));
          return;
        }
        if (!EnrollGuard.sampleTrusted(selfMatch, tempId)) {
          await _discardTemp(tempId);
          _snack('Face sample was too poor to trust - retake in better '
              'light. Nothing was saved.');
          return;
        }
        // Promote: true replacement is delete-then-register, done LAST so
        // a failed promotion can never cost the old sample. The engine has
        // already processed these exact bytes, so a throw here is rare.
        try {
          await widget.face.forget(enrollId);
          await widget.face.register(
            studentId: enrollId,
            imagePath: shotPath,
            name: enrollName,
          );
        } on PlatformException catch (error) {
          _logEngineFailure('promote', error);
          await _discardTemp(tempId);
          _snack('Enrollment could not be saved: ${EnrollErrors.message(error)}');
          return;
        }
        await _discardTemp(tempId);
        await widget.store.addStudent(
          Student(
            id: enrollId,
            name: enrollName,
            createdAt: DateTime.now().toUtc(),
          ),
        );
        _nameController.clear();
        _idController.clear();
        await widget.onChanged();
        await _reload();
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('$enrollName enrolled')),
          );
        }
        return;
      }
    } catch (error) {
      _snack('Enrollment failed: ${EnrollErrors.message(error)}');
    } finally {
      if (shotPath != null) {
        try {
          await File(shotPath).delete();
        } catch (_) {}
      }
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _discardTemp(String tempId) async {
    try {
      await widget.face.forget(tempId);
    } catch (_) {}
  }

  /// Full engine failures go to the log for diagnosis; the operator only
  /// ever sees the mapped one-liner.
  void _logEngineFailure(String stage, PlatformException error) {
    debugPrint('enroll: face engine failed in $stage: '
        '${error.code} ${error.message}');
  }

  /// The identify pass matched the capture to an already-enrolled student.
  /// Returns true = replace that student's sample, false = enroll as a NEW
  /// student anyway, null = cancel.
  Future<bool?> _confirmDuplicate(Student matched) async {
    return showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Face already enrolled'),
        content: Text(
          'This face matches ${matched.name}, who is already enrolled. '
          'Replace their stored face sample, or enroll it as a NEW student '
          'anyway (not recommended - the door will keep recognizing the '
          'older sample).',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(null),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('Enroll as new anyway'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: Text("Replace ${matched.name}'s sample"),
          ),
        ],
      ),
    );
  }

  /// Asks the operator before overwriting an existing student's face sample.
  Future<bool> _confirmReplace(Student existing, String id) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: const Text('Replace face sample?'),
        content: Text(
          '${existing.name} ($id) is already enrolled. Capturing again '
          'replaces their stored face sample (attendance history is kept).',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: const Text('Replace'),
          ),
        ],
      ),
    );
    return confirmed == true;
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

  /// Mints and displays a parent-pairing code for [student]. The mint future
  /// is computed ONCE here (review G2): a FutureBuilder inside the dialog
  /// builder would re-fire the request if the route ever rebuilt, silently
  /// minting a second code.
  Future<void> _showPairingCode(Student student) async {
    final service = widget.codeService ?? CodeService();
    final mintFuture = service.mint(student.id);
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text('Pairing code - ${student.name}'),
        content: FutureBuilder<PairingCodeInfo?>(
          future: mintFuture,
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
              helperText:
                  'Ids are internal - leave empty and one is generated; '
                  'nobody ever needs to type or share it',
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
          TextField(
            controller: _searchController,
            onChanged: (_) => setState(() {}),
            decoration: const InputDecoration(
              prefixIcon: Icon(Icons.search),
              hintText: 'Find an enrolled student by name',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          Builder(builder: (context) {
            final filtered = _filtered;
            final count = filtered.length == _students.length
                ? 'Enrolled (${_students.length})'
                : 'Showing ${filtered.length} of ${_students.length}';
            return Text(count, style: Theme.of(context).textTheme.titleMedium);
          }),
          ..._filtered.map(
            (student) => ListTile(
              leading: const Icon(Icons.face),
              title: Text(student.name),
              // Ids stay internal (name-first flow): no id shown to the
              // operator - identity at the door is the face, and duplicate
              // checks happen automatically on capture.
              trailing: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  IconButton(
                    tooltip: 'Pairing code for the guardians',
                    icon: const Icon(Icons.key),
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
            )
          else if (_filtered.isEmpty)
            Padding(
              padding: const EdgeInsets.all(8),
              child: Text(
                'No enrolled student matches "${_searchController.text.trim()}".',
              ),
            ),
        ],
      ),
    );
  }
}
