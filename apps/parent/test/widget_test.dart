import 'package:flutter_test/flutter_test.dart';

import 'package:parent_app/main.dart';

void main() {
  testWidgets('parent dashboard screen renders', (tester) async {
    await tester.pumpWidget(const ParentApp());

    expect(find.text('Parent - Attendance Dashboard'), findsOneWidget);
  });
}
