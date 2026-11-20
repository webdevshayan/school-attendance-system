// Pairing + guardian-registry persistence on top of the same Turso store.
//
// Schema additions (created idempotently by Init):
//
//	pairing_codes: short codes the door admin mints per student; the school
//	  hands them to guardians out of band. Multi-use within their TTL so both
//	  parents can pair from one code.
//	parent_devices: paired parent installs, keyed by the SHA-256 of their
//	  bearer token (the token itself is never stored server-side; it is
//	  returned exactly once at pairing time).
package store

import (
	"context"

	"github.com/webdevshayan/school-attendance-system/server/pkg/turso"
)

// PairingCode is one minted code for pairing a guardian install to a student.
type PairingCode struct {
	Code      string
	StudentID string
	CreatedAt string // RFC3339
	ExpiresAt string // RFC3339; expiry is enforced by the handler
}

// ParentDevice is one paired parent install.
type ParentDevice struct {
	TokenHash string // hex SHA-256 of the bearer token
	StudentID string
	Label     string
	Platform  string
	PairedAt  string // RFC3339
}

// EventRecord is one attendance row as delivered to the parent side.
type EventRecord struct {
	ID         int64 // autoincrement; doubles as the polling cursor
	DeviceID   string
	StudentID  string
	Status     string
	MarkedAt   string
	ReceivedAt string
}

// Store gains the pairing/registry/events operations (one interface for the
// whole store keeps the handler fakes single-purpose).
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

	// MintCode upserts a pairing code for a student.
	MintCode(ctx context.Context, c PairingCode) error
	// FindCode returns a minted code record (ok=false when unknown).
	FindCode(ctx context.Context, code string) (PairingCode, bool, error)
	// UpsertParentDevice registers or refreshes a paired parent install.
	UpsertParentDevice(ctx context.Context, d ParentDevice) error
	// StudentForToken resolves a parent bearer token (by hash) to the
	// student the install is paired with ("" when unknown).
	StudentForToken(ctx context.Context, tokenHash string) (string, error)
	// EventsSince returns up to limit attendance rows for a student with
	// autoincrement id > sinceID, oldest first.
	EventsSince(ctx context.Context, studentID string, sinceID int64, limit int) ([]EventRecord, error)
}

// Init in store.go creates the pairing tables in the same pipeline request
// as the attendance tables; this file only carries the operations.

// MintCode upserts a pairing code (regenerating extends the TTL).
func (s *TursoStore) MintCode(ctx context.Context, c PairingCode) error {
	_, err := s.c.Execute(ctx, turso.Statement{
		SQL: `INSERT INTO pairing_codes (code, student_id, created_at, expires_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (code) DO UPDATE SET student_id = excluded.student_id, expires_at = excluded.expires_at`,
		Args: []turso.Arg{
			turso.Text(c.Code), turso.Text(c.StudentID),
			turso.Text(c.CreatedAt), turso.Text(c.ExpiresAt),
		},
	})
	return err
}

// FindCode returns a minted code record; ok=false when the code is unknown.
func (s *TursoStore) FindCode(ctx context.Context, code string) (PairingCode, bool, error) {
	results, err := s.c.Execute(ctx, turso.Statement{
		SQL:  `SELECT student_id, created_at, expires_at FROM pairing_codes WHERE code = ?`,
		Args: []turso.Arg{turso.Text(code)},
	})
	if err != nil {
		return PairingCode{}, false, err
	}
	rows := results[0].Rows
	if len(rows) == 0 || len(rows[0]) < 3 {
		return PairingCode{}, false, nil
	}
	return PairingCode{
		Code:      code,
		StudentID: rows[0][0].Str(),
		CreatedAt: rows[0][1].Str(),
		ExpiresAt: rows[0][2].Str(),
	}, true, nil
}

// UpsertParentDevice registers a paired install or refreshes its binding.
func (s *TursoStore) UpsertParentDevice(ctx context.Context, d ParentDevice) error {
	_, err := s.c.Execute(ctx, turso.Statement{
		SQL: `INSERT INTO parent_devices (token_hash, student_id, label, platform, paired_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (token_hash) DO UPDATE SET
  student_id = excluded.student_id,
  label = excluded.label,
  platform = excluded.platform,
  paired_at = excluded.paired_at`,
		Args: []turso.Arg{
			turso.Text(d.TokenHash), turso.Text(d.StudentID),
			turso.Text(d.Label), turso.Text(d.Platform),
			turso.Text(d.PairedAt),
		},
	})
	return err
}

// StudentForToken resolves a bearer token (by hash) to its student; "" when
// the token is unknown.
func (s *TursoStore) StudentForToken(ctx context.Context, tokenHash string) (string, error) {
	results, err := s.c.Execute(ctx, turso.Statement{
		SQL:  `SELECT student_id FROM parent_devices WHERE token_hash = ?`,
		Args: []turso.Arg{turso.Text(tokenHash)},
	})
	if err != nil {
		return "", err
	}
	rows := results[0].Rows
	if len(rows) == 0 || len(rows[0]) == 0 {
		return "", nil
	}
	return rows[0][0].Str(), nil
}

// EventsSince returns up to limit attendance rows for a student with
// autoincrement id > sinceID, oldest first (polling cursor order).
func (s *TursoStore) EventsSince(ctx context.Context, studentID string, sinceID int64, limit int) ([]EventRecord, error) {
	results, err := s.c.Execute(ctx, turso.Statement{
		SQL: `SELECT id, device_id, student_id, status, marked_at, received_at
FROM attendance_rows
WHERE student_id = ? AND id > ?
ORDER BY id
LIMIT ?`,
		Args: []turso.Arg{turso.Text(studentID), turso.Integer(sinceID), turso.Integer(int64(limit))},
	})
	if err != nil {
		return nil, err
	}
	events := make([]EventRecord, 0, results[0].RowCount)
	for _, row := range results[0].Rows {
		if len(row) < 6 {
			continue
		}
		events = append(events, EventRecord{
			ID:         row[0].Int(),
			DeviceID:   row[1].Str(),
			StudentID:  row[2].Str(),
			Status:     row[3].Str(),
			MarkedAt:   row[4].Str(),
			ReceivedAt: row[5].Str(),
		})
	}
	return events, nil
}
