import 'package:path_provider/path_provider.dart';
import 'package:sqflite/sqflite.dart';

/// Key-value persistence for the pairing state: which student this install
/// follows, its bearer token and the events-feed cursor.
///
/// Tests inject an in-memory fake; the sqflite implementation keeps the
/// pairing state in its own tiny database so the attendance cache stays
/// untouched.
abstract class PairingPreferences {
  Future<String?> read(String key);
  Future<void> write(String key, String value);
  Future<void> clear();
}

/// Preference keys used by [PairingService] and [EventPoller].
class PairingKeys {
  PairingKeys._();

  static const studentId = 'student_id';
  static const token = 'token';
  static const cursor = 'cursor';
}

/// sqflite-backed preferences in parent_pairing.db (settings table).
class SqflitePairingPreferences implements PairingPreferences {
  Database? _db;

  Future<Database> _open() async {
    final existing = _db;
    if (existing != null) return existing;
    final dir = await getApplicationDocumentsDirectory();
    final opened = await openDatabase(
      '${dir.path}/parent_pairing.db',
      version: 1,
      onCreate: (database, version) => database.execute(
        'CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)',
      ),
    );
    _db = opened;
    return opened;
  }

  @override
  Future<String?> read(String key) async {
    final database = await _open();
    final rows = await database.query(
      'settings',
      columns: ['value'],
      where: 'key = ?',
      whereArgs: [key],
      limit: 1,
    );
    if (rows.isEmpty) return null;
    return rows.first['value'] as String?;
  }

  @override
  Future<void> write(String key, String value) async {
    final database = await _open();
    await database.insert(
      'settings',
      {'key': key, 'value': value},
      conflictAlgorithm: ConflictAlgorithm.replace,
    );
  }

  @override
  Future<void> clear() async {
    final database = await _open();
    await database.delete('settings');
    _db = null;
  }
}

/// In-memory implementation for tests and degraded runs.
class InMemoryPairingPreferences implements PairingPreferences {
  final Map<String, String> _values = {};

  @override
  Future<String?> read(String key) async => _values[key];

  @override
  Future<void> write(String key, String value) async => _values[key] = value;

  @override
  Future<void> clear() async => _values.clear();
}
