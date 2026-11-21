import 'package:face_verification/face_verification.dart';

/// Thin wrapper around the face_verification plugin so the rest of the app
/// depends on a small, readable surface instead of the plugin singleton.
/// The FaceNet TFLite model ships inside the plugin - no downloads needed.
class FaceService {
  FaceService({this.threshold = 0.70});

  /// Cosine-similarity threshold above which a probe face counts as a match.
  final double threshold;

  /// Loads the bundled FaceNet model. Call once at startup.
  Future<void> init() => FaceVerification.instance.init();

  Future<bool> isRegistered(String studentId) =>
      FaceVerification.instance.isFaceRegistered(studentId);

  /// Registers the captured photo as this student's door sample.
  ///
  /// The plugin throws 'A face record with ID ... and imageId ... already
  /// exists' for a repeated (id, imageId) pair BEFORE it honors `replace` -
  /// a constant imageId would brick every re-enrollment. A unique per-call
  /// imageId sidesteps that; true replacement is delete-then-register at
  /// the call site, ordered non-destructively.
  Future<void> register({
    required String studentId,
    required String imagePath,
    required String name,
  }) =>
      FaceVerification.instance.registerFromImagePath(
        id: studentId,
        imagePath: imagePath,
        imageId: 'enroll_${DateTime.now().millisecondsSinceEpoch}',
        name: name,
        replace: true,
      );

  /// Returns the matched student id, or null when nobody matches.
  /// Runs in a background isolate so the preview stays smooth.
  Future<String?> verify(String imagePath) =>
      FaceVerification.instance.verifyFromImagePathIsolate(
        imagePath: imagePath,
        threshold: threshold,
      );

  Future<void> forget(String studentId) =>
      FaceVerification.instance.deleteUserFaces(studentId);

  Future<void> dispose() => FaceVerification.instance.dispose();
}
