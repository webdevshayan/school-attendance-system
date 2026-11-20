import 'package:flutter/material.dart';

import 'ui/dashboard_page.dart';

void main() => runApp(const ParentApp());

/// Parent-facing app for the school attendance system.
///
/// Phase 3 wires the engaging dashboard (today's status, event timeline,
/// weekly summary) fed by a local cache; push handling is transport-agnostic
/// and Phase 4 connects it to the server's fan-out.
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
      home: const DashboardPage(),
    );
  }
}
