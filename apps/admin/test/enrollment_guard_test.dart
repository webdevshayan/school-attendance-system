import 'package:flutter_test/flutter_test.dart';

import 'package:admin_app/core/db/attendance_store.dart';
import 'package:admin_app/core/recognition/enrollment_guard.dart';

void main() {
  group('EnrollGuard.sampleTrusted', () {
    test('a sample that recognizes itself back as the same id is trusted', () {
      expect(EnrollGuard.sampleTrusted('s_123', 's_123'), isTrue);
    });

    test('no match -> untrusted', () {
      expect(EnrollGuard.sampleTrusted(null, 's_123'), isFalse);
    });

    test('a DIFFERENT id matching back is a mis-enrollment, not a success', () {
      expect(EnrollGuard.sampleTrusted('s_other', 's_123'), isFalse);
    });
  });

  group('EnrollGuard.findExisting', () {
    final students = [
      Student(id: 's_1', name: 'Aisha', createdAt: DateTime.utc(2026, 11, 1)),
      Student(id: 's_2', name: 'Bilal', createdAt: DateTime.utc(2026, 11, 2)),
    ];

    test('an id that already exists is found (replace flow)', () {
      expect(EnrollGuard.findExisting(students, 's_2')?.name, 'Bilal');
    });

    test('unknown id -> null (fresh enroll)', () {
      expect(EnrollGuard.findExisting(students, 's_999'), isNull);
    });

    test('generated ids are checked like any other', () {
      expect(EnrollGuard.findExisting(students, 's_1')?.id, 's_1');
    });
  });
}
