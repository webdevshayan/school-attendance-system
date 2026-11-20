import 'package:admin_app/core/attendance/attendance_pipeline.dart';
import 'package:flutter_test/flutter_test.dart';

import 'fake_store.dart';

void main() {
  test('first sight records attendance, immediate repeat hits cooldown',
      () async {
    final store = FakeStore();
    var now = DateTime(2026, 11, 20, 8, 0);
    final pipeline = AttendancePipeline(
      store: store,
      deviceId: 'door-1',
      cooldown: const Duration(minutes: 3),
      now: () => now,
    );

    final first = await pipeline.recordMatch(
      studentId: 's1',
      studentName: 'Ali',
      status: 'in',
    );
    expect(first.decision, PipelineDecision.recorded);
    expect(first.record!.synced, isFalse);
    expect(first.record!.deviceId, 'door-1');
    expect(store.rows.length, 1);

    now = now.add(const Duration(seconds: 30));
    final second = await pipeline.recordMatch(
      studentId: 's1',
      studentName: 'Ali',
      status: 'in',
    );
    expect(second.decision, PipelineDecision.cooldownActive);
    expect(store.rows.length, 1);
  });

  test('same status blocked for the rest of the UTC day, next day works',
      () async {
    final store = FakeStore();
    var now = DateTime(2026, 11, 20, 8, 0);
    final pipeline = AttendancePipeline(
      store: store,
      deviceId: 'door-1',
      cooldown: const Duration(minutes: 3),
      now: () => now,
    );

    expect(
      (await pipeline.recordMatch(
        studentId: 's1',
        studentName: 'Ali',
        status: 'in',
      ))
          .decision,
      PipelineDecision.recorded,
    );

    // After the cooldown the day-level rule still blocks a second 'in'.
    now = now.add(const Duration(minutes: 10));
    expect(
      (await pipeline.recordMatch(
        studentId: 's1',
        studentName: 'Ali',
        status: 'in',
      ))
          .decision,
      PipelineDecision.alreadyMarkedToday,
    );

    // A new UTC day unlocks the mark again.
    now = DateTime(2026, 11, 21, 8, 0);
    final nextDay = await pipeline.recordMatch(
      studentId: 's1',
      studentName: 'Ali',
      status: 'in',
    );
    expect(nextDay.decision, PipelineDecision.recorded);
    expect(store.rows.length, 2);
  });

  test('in and out on the same day are independent', () async {
    final store = FakeStore();
    var now = DateTime(2026, 11, 20, 8, 0);
    final pipeline = AttendancePipeline(
      store: store,
      deviceId: 'door-1',
      cooldown: const Duration(minutes: 3),
      now: () => now,
    );

    expect(
      (await pipeline.recordMatch(
        studentId: 's1',
        studentName: 'Ali',
        status: 'in',
      ))
          .decision,
      PipelineDecision.recorded,
    );

    now = now.add(const Duration(hours: 6));
    final out = await pipeline.recordMatch(
      studentId: 's1',
      studentName: 'Ali',
      status: 'out',
    );
    expect(out.decision, PipelineDecision.recorded);
    expect(store.rows.map((r) => r.status), ['in', 'out']);
  });

  test('different students do not share cooldown or day rules', () async {
    final store = FakeStore();
    final pipeline = AttendancePipeline(
      store: store,
      deviceId: 'door-1',
      cooldown: const Duration(minutes: 3),
      now: () => DateTime(2026, 11, 20, 8, 0),
    );

    expect(
      (await pipeline.recordMatch(
        studentId: 's1',
        studentName: 'Ali',
        status: 'in',
      ))
          .decision,
      PipelineDecision.recorded,
    );
    expect(
      (await pipeline.recordMatch(
        studentId: 's2',
        studentName: 'Sara',
        status: 'in',
      ))
          .decision,
      PipelineDecision.recorded,
    );
    expect(store.rows.length, 2);
  });
}
