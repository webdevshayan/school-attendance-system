import 'package:flutter_test/flutter_test.dart';

import 'package:admin_app/main.dart';

void main() {
  testWidgets('admin capture screen renders', (tester) async {
    await tester.pumpWidget(const AdminApp());

    expect(find.text('Admin - Attendance Capture'), findsOneWidget);
  });
}
