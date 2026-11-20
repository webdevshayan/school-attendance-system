import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:admin_app/core/pairing/code_service.dart';

const testSecret = 'phase5-test-secret';

void main() {
  CodeService serviceWith(MockClient client) => CodeService(
        client: client,
        baseUrl: 'http://test.local',
        deviceId: 'door-1',
        signingSecret: testSecret,
      );

  test('mint posts the documented signed payload and parses the code',
      () async {
    final requests = <http.Request>[];
    final client = MockClient((request) async {
      requests.add(request);
      return http.Response(
        '{"code":"ABCD2345","student_id":"s1","expires_at":"2026-11-27T14:00:00.000Z"}',
        200,
      );
    });
    final service = serviceWith(client);

    final info = await service.mint('s1');

    expect(info, isNotNull);
    expect(info!.code, 'ABCD2345');
    expect(info.studentId, 's1');
    expect(info.expiresAt, DateTime.utc(2026, 11, 27, 14));

    expect(requests, hasLength(1));
    expect(requests.first.url.path, '/api/codes');
    expect(requests.first.body, '{"student_id":"s1"}');

    // Same signing scheme as /api/sync: the three HMAC headers must be set.
    final headers = requests.first.headers;
    expect(headers['x-device-id'], 'door-1');
    expect(headers['x-sync-timestamp'], isNotNull);
    expect(headers['x-sync-signature'], isNotNull);
  });

  test('a server failure or offline state yields null, not an exception',
      () async {
    expect(
      await serviceWith(MockClient((request) async {
        return http.Response('{"error":"boom"}', 500);
      }))
          .mint('s1'),
      isNull,
    );
    expect(
      await serviceWith(MockClient((request) async {
        throw http.ClientException('offline');
      }))
          .mint('s1'),
      isNull,
    );
  });

  test('unsigned builds and unconfigured URLs never hit the network',
      () async {
    var requests = 0;
    final client = MockClient((request) async {
      requests++;
      return http.Response('{"code":"ABCD2345","student_id":"s1"}', 200);
    });

    // No signing secret baked in (local debug build).
    final unsigned = CodeService(
      client: client,
      baseUrl: 'http://test.local',
      deviceId: 'door-1',
      signingSecret: '',
    );
    expect(await unsigned.mint('s1'), isNull);

    // Placeholder URL.
    final unconfigured = CodeService(
      client: client,
      baseUrl: 'https://school-attendance-system.vercel.app',
      deviceId: 'door-1',
      signingSecret: testSecret,
    );
    expect(await unconfigured.mint('s1'), isNull);

    expect(requests, 0, reason: 'nothing may leave the device');
  });

  test('a malformed server response yields null', () async {
    expect(
      await serviceWith(MockClient((request) async {
        return http.Response('not json at all', 200);
      }))
          .mint('s1'),
      isNull,
    );
    expect(
      await serviceWith(MockClient((request) async {
        return http.Response('{"code":""}', 200);
      }))
          .mint('s1'),
      isNull,
    );
  });
}
