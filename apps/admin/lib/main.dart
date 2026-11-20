import 'package:flutter/material.dart';

import 'ui/admin_home_page.dart';

void main() => runApp(const AdminApp());

/// Door-side admin app for the school attendance system.
///
/// Phase 2 wires the real pipeline: camera preview -> on-device face
/// recognition (`face_verification`, bundled FaceNet TFLite) -> attendance
/// rows in local SQLite (`libsql_dart`) -> background sync to `/api/sync`.
class AdminApp extends StatelessWidget {
  const AdminApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'School Attendance - Admin',
      theme: ThemeData(
        colorSchemeSeed: Colors.indigo,
        useMaterial3: true,
      ),
      home: const AdminHomePage(),
    );
  }
}
