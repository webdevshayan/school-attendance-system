import 'package:flutter_test/flutter_test.dart';

import 'package:admin_app/core/db/attendance_store.dart';
import 'package:admin_app/core/roster/roster_search.dart';

void main() {
  final roster = [
    Student(id: 's_1', name: 'Aisha Khan', createdAt: DateTime.utc(2026, 11, 1)),
    Student(id: 's_2', name: 'Bilal Ahmad', createdAt: DateTime.utc(2026, 11, 2)),
    Student(id: 's_3', name: 'aisha raza', createdAt: DateTime.utc(2026, 11, 3)),
  ];

  group('RosterSearch.filter', () {
    test('a blank query returns every student, order preserved', () {
      final out = RosterSearch.filter(roster, '');
      expect(out.map((s) => s.id), ['s_1', 's_2', 's_3']);
    });

    test('a whitespace-only query behaves like a blank one', () {
      expect(RosterSearch.filter(roster, '   ').length, 3);
    });

    test('an exact full name matches', () {
      expect(RosterSearch.filter(roster, 'Bilal Ahmad').map((s) => s.id),
          ['s_2']);
    });

    test('a mid-name substring matches', () {
      expect(RosterSearch.filter(roster, 'mad').map((s) => s.id), ['s_2']);
    });

    test('matching is case-insensitive in both directions', () {
      expect(RosterSearch.filter(roster, 'AISHA').length, 2);
      expect(RosterSearch.filter(roster, 'razA').map((s) => s.id), ['s_3']);
    });

    test('no match -> empty list (the page shows its own hint)', () {
      expect(RosterSearch.filter(roster, 'zahra'), isEmpty);
    });

    test('internal ids are NOT searchable (they stay machine keys)', () {
      expect(RosterSearch.filter(roster, 's_1'), isEmpty);
    });

    test('a returned blank-query list is a copy, not the input', () {
      final out = RosterSearch.filter(roster, '');
      expect(identical(out, roster), isFalse);
    });
  });
}
