/// Maps face-engine errors to operator guidance. The full exception (with
/// its native stack) goes to the debug log, never to the operator - door
/// staff needs the next action, not an obfuscated stack trace.
class EnrollErrors {
  const EnrollErrors._();

  /// One line of "what happened + what to do now" for the snackbar.
  static String message(Object error) {
    final raw = error.toString().toLowerCase();
    if (raw.contains('no face detected')) {
      return 'No face found in the shot - center the face, 0.5-1.5 m away, '
          'in good light, then retake.';
    }
    if (raw.contains('multiple faces')) {
      return 'More than one face in the shot - enroll one student at a '
          'time, then retake.';
    }
    if (raw.contains('platformexception') ||
        raw.contains('nullpointerexception') ||
        raw.contains('mlkit') ||
        raw.contains('detector')) {
      // ML Kit's Java side can NPE on a bad frame (or while the device's
      // face module is still being provisioned). Retrying with a fresh
      // capture fixes the transient cases.
      return 'The face engine failed on this capture. Retake once; if it '
          'keeps failing, update Google Play services and retry in a '
          'minute.';
    }
    if (raw.contains('already exists')) {
      return 'This student already has this exact sample - retake or '
          'remove them first.';
    }
    if (raw.contains('database') || raw.contains('sqflite')) {
      return 'Local storage hiccup - free up space on this device and '
          'retry.';
    }
    return 'Enrollment failed on this capture - retake. If it keeps '
        'failing, restart the app.';
  }
}
