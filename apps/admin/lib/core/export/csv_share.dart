import 'dart:io';

import 'package:path_provider/path_provider.dart';
import 'package:share_plus/share_plus.dart';

import '../db/attendance_store.dart';
import 'attendance_csv.dart';

/// Exports the whole attendance register to a CSV file and hands it to the
/// Android share sheet (Save to Files, WhatsApp, email - the school picks).
/// Runs on demand from the door home screen; failures surface to the caller
/// so the UI can snack-bar them without touching the capture loop.
Future<int> exportAttendanceCsv(AttendanceStore store) async {
  // Window: the register's beginning of time through tomorrow UTC, so
  // today's marks are inside regardless of timezone math.
  final start = DateTime.utc(2000);
  final end = DateTime.now().toUtc().add(const Duration(days: 1));
  final records = await store.attendanceBetween(start, end);
  final csv = AttendanceCsv.build(records);

  final dir = await getTemporaryDirectory();
  final stamp = DateTime.now()
      .toUtc()
      .toIso8601String()
      .replaceAll(RegExp('[:.]'), '-');
  final file = File('${dir.path}/attendance_register_$stamp.csv');
  await file.writeAsString(csv, flush: true);

  await Share.shareXFiles(
    [XFile(file.path)],
    subject: 'Attendance register',
    text: 'Attendance register - ${records.length} marks',
  );
  return records.length;
}
