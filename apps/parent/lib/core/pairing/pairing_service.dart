import 'dart:convert';

import 'package:http/http.dart' as http;

import '../config.dart';
import 'pairing_preferences.dart';

/// Redeems a pairing code against POST /api/pair and persists the binding
/// (student id + bearer token) on this install. The token is returned by the
/// server exactly once and lives only in the local pairing preferences.
class PairingService {
  PairingService({
    required PairingPreferences prefs,
    http.Client? client,
    this.baseUrl = ChildConfig.serverBaseUrl,
  })  : _prefs = prefs,
        _client = client ?? http.Client();

  final PairingPreferences _prefs;
  final http.Client _client;
  final String baseUrl;

  /// True once this install holds a token (a binding may still have been
  /// revoked server-side; the poller detects that via 401).
  Future<bool> isPaired() async {
    final token = await _prefs.read(PairingKeys.token);
    return token != null && token.isNotEmpty;
  }

  /// The student this install follows, or null while unpaired.
  Future<String?> pairedStudentId() async {
    final id = await _prefs.read(PairingKeys.studentId);
    if (id == null || id.isEmpty) return null;
    return id;
  }

  Future<String?> token() async {
    final token = await _prefs.read(PairingKeys.token);
    if (token == null || token.isEmpty) return null;
    return token;
  }

  /// Redeems [code]. Returns true on success (binding persisted), false on
  /// any failure (unknown/expired code, server problem, offline).
  Future<bool> pair(String code) async {
    if (baseUrl == ChildConfig.unconfiguredServerUrl) return false;
    try {
      final response = await _client.post(
        Uri.parse('$baseUrl/api/pair'),
        headers: const {'Content-Type': 'application/json'},
        body: jsonEncode(<String, dynamic>{
          'code': code.trim().toUpperCase(),
          'label': 'parent install',
          'platform': 'android',
        }),
      );
      if (response.statusCode < 200 || response.statusCode >= 300) {
        return false;
      }
      final decoded = jsonDecode(response.body);
      if (decoded is! Map<String, dynamic>) return false;
      final studentId = decoded['student_id'];
      final token = decoded['token'];
      if (studentId is! String || studentId.isEmpty) return false;
      if (token is! String || token.isEmpty) return false;
      await _prefs.write(PairingKeys.studentId, studentId);
      await _prefs.write(PairingKeys.token, token);
      return true;
    } catch (_) {
      // Offline or transport error: caller stays unpaired and can retry.
      return false;
    }
  }

  /// Forgets the binding on this install (revocation detected server-side).
  Future<void> unpair() => _prefs.clear();
}
