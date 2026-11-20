import 'package:flutter_test/flutter_test.dart';

import 'package:admin_app/main.dart';

void main() {
  testWidgets('admin home renders and degrades gracefully without plugins',
      (tester) async {
    await tester.pumpWidget(const AdminApp());

    // Let the async bootstrap hit the (absent) plugins and settle into the
    // degraded state. No pumpAndSettle: the ready state owns periodic timers.
    await tester.pump(const Duration(seconds: 1));

    expect(find.text('School Attendance - Admin'), findsOneWidget);
    expect(find.text('Retry'), findsOneWidget);
  });
}
