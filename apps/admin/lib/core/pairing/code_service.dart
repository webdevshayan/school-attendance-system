import 'dart:convert';

import 'package:http/http.dart' as http;

import '../config.dart';
import '../sync/request_signer.dart';

/// A successfully minted pairing code.
class PairingCodeInfo {
  const PairingCodeInfo({
    required this.code,
    required this.studentId,
    required this.expiresAt,
  });

  final String code;
  final String studentId;
  final DateTime expiresAt;
}

/// Mints parent-pairing codes via POST /api/codes.
///
/// Signed exactly like /api/sync (same HMAC headers): only an allowlisted
/// door device can hand out codes, so a stolen phone cannot invite itself.
/// Returns null on any failure - callers show a friendly retry hint instead
/// of a stack trace; the operation is always safe to retry.
class CodeService {
  CodeService({
    http.Client? client,
    this.baseUrl = AdminConfig.serverBaseUrl,
    this.deviceId = AdminConfig.deviceId,
    this.signingSecret = AdminConfig.syncSigningSecret,
  }) : _client = client ?? http.Client();

  final http.Client _client;
  final String baseUrl;
  final String deviceId;
  final String signingSecret;

  Future<PairingCodeInfo?> mint(String studentId) async {
    // Unsigned local builds and unconfigured URLs never touch the network.
    if (baseUrl == AdminConfig.unconfiguredServerUrl) return null;
    if (signingSecret.isEmpty) return null;
    final body = jsonEncode(<String, dynamic>{'student_id': studentId});
    try {
      final response = await _client.post(
        Uri.parse('$baseUrl/api/codes'),
        headers: signedHeaders(
          deviceId: deviceId,
          secret: signingSecret,
          body: body,
        ),
        body: body,
      );
      if (response.statusCode < 200 || response.statusCode >= 300) {
        return null;
      }
      final decoded = jsonDecode(response.body);
      if (decoded is! Map<String, dynamic>) return null;
      final code = decoded['code'];
      final returnedStudent = decoded['student_id'];
      final expiresAt = decoded['expires_at'];
      if (code is! String || code.isEmpty) return null;
      if (returnedStudent is! String || returnedStudent.isEmpty) return null;
      if (expiresAt is! String) return null;
      return PairingCodeInfo(
        code: code,
        studentId: returnedStudent,
        expiresAt: DateTime.parse(expiresAt),
      );
    } catch (_) {
      // Offline or transport error: caller retries later.
      return null;
    }
  }
}
