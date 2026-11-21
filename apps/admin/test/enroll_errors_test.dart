import 'package:flutter_test/flutter_test.dart';

import 'package:admin_app/core/recognition/enroll_errors.dart';

void main() {
  group('EnrollErrors.message', () {
    test('no face detected -> framing guidance', () {
      final message = EnrollErrors.message(Exception('No face detected'));
      expect(message, contains('center the face'));
      expect(message, isNot(contains('Exception')));
    });

    test('multiple faces -> one-at-a-time guidance', () {
      final message =
          EnrollErrors.message(Exception('Multiple faces detected'));
      expect(message, contains('one student at a time'));
    });

    test('native ML Kit NPE -> face-engine guidance, never the stack', () {
      const npe = 'PlatformException(error, Attempt to invoke virtual '
          'method \'java.lang.Class java.lang.Object.getClass()\' on a null '
          'object reference, null, java.lang.NullPointerException: ...)';
      final message = EnrollErrors.message(npe);
      expect(message, contains('face engine failed'));
      expect(message, isNot(contains('java.lang')));
      expect(message, isNot(contains('PlatformException')));
    });

    test('duplicate sample -> retake-or-remove guidance', () {
      final message = EnrollErrors.message(Exception(
          'A face record with ID "s_1" and imageId "x" already exists.'));
      expect(message, contains('already has this exact sample'));
    });

    test('storage failure -> space guidance', () {
      final message =
          EnrollErrors.message(Exception('sqflite disk full'));
      expect(message, contains('free up space'));
    });

    test('anything else -> generic retake guidance', () {
      expect(EnrollErrors.message(StateError('weird')), contains('retake'));
    });
  });
}
