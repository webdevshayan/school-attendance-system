// Package store persists attendance rows and sync devices to Turso (libSQL).
//
// Schema (both tables are created idempotently by Init):
//
//	attendance_rows: one row per mark, de-duplicated server-side on
//	  (device_id, student_id, status, marked_at) so a lost-ack retry from
//	  the admin app cannot double-record attendance.
//	sync_devices:    the allowlist of door devices; only registered devices
//	  may post to /api/sync.
package store

import (
	"context"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/pkg/turso"
)

// Row is one attendance mark as received from an admin device.
type Row struct {
	DeviceID  string
	StudentID string
	Status    string // "in" or "out"
	MarkedAt  string // RFC3339, device clock converted to UTC by the app
}

// Device is a registered door-side device.
type Device struct {
	DeviceID string
	Label    string
}

// Store is the persistence seam; handlers depend on this interface so tests
// can run without a database.
type Store interface {
	// Init creates the schema. Safe to call on every cold start.
	Init(ctx context.Context) error
	// InsertRow stores one attendance mark. It returns false when the exact
	// row (device, student, status, instant) already exists.
	InsertRow(ctx context.Context, r Row) (bool, error)
	// UpsertDevice registers or refreshes a device.
	UpsertDevice(ctx context.Context, d Device) error
	// DeviceExists reports whether the device is on the allowlist.
	DeviceExists(ctx context.Context, deviceID string) (bool, error)
}

// TursoStore is the Turso-backed Store implementation.
type TursoStore struct {
	c    *turso.Client
	Now  func() time.Time // injectable clock (received_at / registered_at)
	init bool             // schema bootstrap memo (functions are warm per instance)
}

// NewTurso wraps a Turso client.
func NewTurso(c *turso.Client) *TursoStore {
	return &TursoStore{c: c, Now: func() time.Time { return time.Now().UTC() }}
}

// Init creates the schema; the CREATE TABLE IF NOT EXISTS pair runs in one
// pipeline request, so it is one round-trip and idempotent.
func (s *TursoStore) Init(ctx context.Context) error {
	if s.init {
		return nil
	}
	_, err := s.c.Execute(ctx,
		turso.Statement{SQL: `CREATE TABLE IF NOT EXISTS attendance_rows (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id TEXT NOT NULL,
  student_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('in','out')),
  marked_at TEXT NOT NULL,
  received_at TEXT NOT NULL,
  UNIQUE (device_id, student_id, status, marked_at)
)`},
		turso.Statement{SQL: `CREATE TABLE IF NOT EXISTS sync_devices (
  device_id TEXT PRIMARY KEY,
  label TEXT NOT NULL DEFAULT '',
  registered_at TEXT NOT NULL
)`},
	)
	if err == nil {
		s.init = true
	}
	return err
}

// InsertRow stores one attendance mark; ON CONFLICT DO NOTHING turns a
// lost-ack retry into a no-op, reported as inserted=false.
func (s *TursoStore) InsertRow(ctx context.Context, r Row) (bool, error) {
	results, err := s.c.Execute(ctx, turso.Statement{
		SQL: `INSERT INTO attendance_rows (device_id, student_id, status, marked_at, received_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING`,
		Args: []turso.Arg{
			turso.Text(r.DeviceID), turso.Text(r.StudentID),
			turso.Text(r.Status), turso.Text(r.MarkedAt),
			turso.Text(s.Now().Format(time.RFC3339)),
		},
	})
	if err != nil {
		return false, err
	}
	return results[0].AffectedRowCount > 0, nil
}

// UpsertDevice registers a device or refreshes its label.
func (s *TursoStore) UpsertDevice(ctx context.Context, d Device) error {
	_, err := s.c.Execute(ctx, turso.Statement{
		SQL: `INSERT INTO sync_devices (device_id, label, registered_at)
VALUES (?, ?, ?)
ON CONFLICT (device_id) DO UPDATE SET label = excluded.label`,
		Args: []turso.Arg{turso.Text(d.DeviceID), turso.Text(d.Label), turso.Text(s.Now().Format(time.RFC3339))},
	})
	return err
}

// DeviceExists checks the allowlist.
func (s *TursoStore) DeviceExists(ctx context.Context, deviceID string) (bool, error) {
	results, err := s.c.Execute(ctx, turso.Statement{
		SQL:  `SELECT 1 FROM sync_devices WHERE device_id = ? LIMIT 1`,
		Args: []turso.Arg{turso.Text(deviceID)},
	})
	if err != nil {
		return false, err
	}
	// SELECTs report affected_row_count 0; the answer is in the returned rows.
	return results[0].RowCount > 0, nil
}
