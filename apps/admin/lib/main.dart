import 'package:flutter/material.dart';

void main() => runApp(const AdminApp());

/// Door-side admin app for the school attendance system.
///
/// Phase 1 is a build-verified scaffold. Phase 2 wires in:
///  * the camera preview pointed at the door,
///  * on-device face recognition (`face_verification`, TFLite),
///  * offline attendance writes into local SQLite (`libsql_dart`),
///  * opportunistic sync to /server whenever connectivity returns.
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
      home: Scaffold(
        appBar: AppBar(title: const Text('School Attendance - Admin')),
        body: const Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.face, size: 72),
              SizedBox(height: 16),
              Text(
                'Admin - Attendance Capture',
                style: TextStyle(fontSize: 22, fontWeight: FontWeight.bold),
              ),
              SizedBox(height: 8),
              Text(
                'Phase 2 lands the camera, on-device face recognition '
                'and the offline attendance queue.',
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
