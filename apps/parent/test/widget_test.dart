import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:parent_app/main.dart';

void main() {
  testWidgets(
      'parent dashboard renders without crashing when plugins are absent',
      (tester) async {
    await tester.pumpWidget(const ParentApp());

    // Bootstrap runs async; successive pumps let each await hop in the
    // storage chain fail and settle. Depending on how the unmocked plugin
    // channels fail inside the test VM, the app ends up either still
    // booting (spinner) or degraded (Retry) - both are valid non-ready
    // states and neither may crash.
    await tester.pump();
    await tester.pump(const Duration(seconds: 1));
    await tester.pump(const Duration(seconds: 1));

    expect(find.text('School Attendance - Parent'), findsOneWidget);
    final settled = find.text('Retry').evaluate().isNotEmpty ||
        find.byType(CircularProgressIndicator).evaluate().isNotEmpty;
    expect(settled, isTrue);
  });
}
