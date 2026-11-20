import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:parent_app/core/pairing/pairing_preferences.dart';
import 'package:parent_app/core/pairing/pairing_service.dart';

final testToken64 = 'f' * 64;

void main() {
  PairingService serviceWith(MockClient client) => PairingService(
        prefs: InMemoryPairingPreferences(),
        client: client,
        baseUrl: 'http://test.local',
      );

  test('a valid code binds the install and persists the token', () async {
    final requests = <http.Request>[];
    final client = MockClient((request) async {
      requests.add(request);
      return http.Response(
          '{"student_id":"s1","token":"$testToken64"}', 200);
    });
    final service = serviceWith(client);

    expect(await service.isPaired(), isFalse);
    expect(await service.pairedStudentId(), isNull);

    final ok = await service.pair('abcd2345');

    expect(ok, isTrue);
    expect(await service.isPaired(), isTrue);
    expect(await service.pairedStudentId(), 's1');
    expect((await service.token())!.length, 64);

    expect(requests, hasLength(1));
    expect(requests.first.url.path, '/api/pair');
    expect(requests.first.body, contains('ABCD2345'),
        reason: 'codes are upper-cased before sending');
  });

  test('an unknown or expired code leaves the install unpaired', () async {
    final client = MockClient((request) async {
      return http.Response('{"error":"unknown code"}', 400);
    });
    final service = serviceWith(client);

    expect(await service.pair('ZZZZ9999'), isFalse);
    expect(await service.isPaired(), isFalse);
    expect(await service.pairedStudentId(), isNull);
  });

  test('offline redemption fails quietly and can be retried', () async {
    final client = MockClient((request) async {
      throw http.ClientException('offline');
    });
    final service = serviceWith(client);

    expect(await service.pair('ABCD2345'), isFalse);
    expect(await service.isPaired(), isFalse);

    // Later, connectivity returns and the same code works.
    final good = MockClient((request) async {
      return http.Response(
          '{"student_id":"s1","token":"$testToken64"}', 200);
    });
    final retry = PairingService(
      prefs: InMemoryPairingPreferences(),
      client: good,
      baseUrl: 'http://test.local',
    );
    expect(await retry.pair('ABCD2345'), isTrue);
  });

  test('unpair forgets the binding', () async {
    final client = MockClient((request) async {
      return http.Response(
          '{"student_id":"s1","token":"$testToken64"}', 200);
    });
    final service = serviceWith(client);
    await service.pair('ABCD2345');
    expect(await service.isPaired(), isTrue);

    await service.unpair();

    expect(await service.isPaired(), isFalse);
    expect(await service.pairedStudentId(), isNull);
  });
}
