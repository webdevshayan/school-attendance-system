import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:admin_app/main.dart';

void main() {
  testWidgets('admin home renders without crashing when plugins are absent',
      (tester) async {
    await tester.pumpWidget(const AdminApp());

    // Bootstrap runs async; successive pumps let each await hop in the
    // face-engine/database/camera chain fail and settle. Depending on how
    // the unmocked plugin channels fail inside the test VM, the app ends up
    // either still booting (spinner) or degraded (Retry) - both are valid
    // non-ready states. It must render and not crash, which is what this
    // test guards; the degraded flow itself is verified on device.
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    await tester.pump(const Duration(seconds: 1));

    expect(find.text('School Attendance - Admin'), findsOneWidget);
    final settled = find.text('Retry').evaluate().isNotEmpty ||
        find.byType(CircularProgressIndicator).evaluate().isNotEmpty;
    expect(settled, isTrue);
  });
}
