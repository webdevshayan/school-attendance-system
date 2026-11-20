import 'package:flutter/material.dart';

void main() => runApp(const ParentApp());

/// Parent-facing app for the school attendance system.
///
/// Phase 1 is a build-verified scaffold. Phase 3 wires in:
///  * Nitroping push notifications when the child's attendance syncs,
///  * the engaging visual attendance dashboard (status, history, streaks).
class ParentApp extends StatelessWidget {
  const ParentApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'School Attendance - Parent',
      theme: ThemeData(
        colorSchemeSeed: Colors.teal,
        useMaterial3: true,
      ),
      home: Scaffold(
        appBar: AppBar(title: const Text('School Attendance - Parent')),
        body: const Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.notifications_active, size: 72),
              SizedBox(height: 16),
              Text(
                'Parent - Attendance Dashboard',
                style: TextStyle(fontSize: 22, fontWeight: FontWeight.bold),
              ),
              SizedBox(height: 8),
              Text(
                'Phase 3 lands the Nitroping push notifications '
                'and the visual attendance dashboard.',
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
