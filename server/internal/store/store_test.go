package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webdevshayan/school-attendance-system/server/internal/turso"
)

// fakeTurso is a scriptable /v2/pipeline server: it records the statements it
// receives and replays one canned result per execute statement (popped in
// order; the last one repeats), so the store's SQL contract is tested without
// a database.
type fakeTurso struct {
	t           *testing.T
	gotSQLs     []string
	gotArgs     [][]string // flat arg values per statement
	results     []stmtResult
	stmtIdx     int // global cursor into results, advances across requests
	calls       int
	lastReqAuth string
}

// stmtResult is the canned outcome for one execute statement.
type stmtResult struct {
	affected int64  // used when errMsg == ""
	rows     int    // used when errMsg == "" && affected < 0 (SELECT shape)
	errMsg   string // non-empty → error result
}

func affectedResult(n int64) stmtResult { return stmtResult{affected: n} }
func rowsResult(n int) stmtResult       { return stmtResult{affected: -1, rows: n} }
func errorResult(msg string) stmtResult { return stmtResult{errMsg: msg} }

func (f *fakeTurso) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls++
		f.lastReqAuth = r.Header.Get("Authorization")
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		var req struct {
			Requests []struct {
				Type string `json:"type"`
				Stmt *struct {
					SQL  string `json:"sql"`
					Args []struct {
						Type  string `json:"type"`
						Value any    `json:"value"`
					} `json:"args"`
				} `json:"stmt"`
			} `json:"requests"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			f.t.Fatalf("bad pipeline request: %v", err)
		}
		execCount := 0
		for _, rq := range req.Requests {
			if rq.Type == "execute" && rq.Stmt != nil {
				f.gotSQLs = append(f.gotSQLs, rq.Stmt.SQL)
				vals := make([]string, 0, len(rq.Stmt.Args))
				for _, a := range rq.Stmt.Args {
					switch v := a.Value.(type) {
					case string:
						vals = append(vals, v)
					default:
						f.t.Fatalf("unexpected arg type %T (%v) - text args must be JSON strings", a.Value, a.Value)
					}
				}
				f.gotArgs = append(f.gotArgs, vals)
				execCount++
			}
		}
		if len(f.results) == 0 {
			f.t.Fatalf("unexpected pipeline call #%d (no canned results)", f.calls)
		}
		results := make([]string, 0, execCount+1)
		for i := 0; i < execCount; i++ {
			// Results are consumed globally across requests: the sequence
			// [insert, insert] spans two pipeline calls with one statement each.
			idx := f.stmtIdx
			f.stmtIdx++
			if idx >= len(f.results) {
				idx = len(f.results) - 1 // repeat the last canned result
			}
			res := f.results[idx]
			switch {
			case res.errMsg != "":
				results = append(results,
					`{"type":"error","error":{"message":`+jsonString(res.errMsg)+`}}`)
			case res.affected >= 0:
				results = append(results, `{"type":"ok","response":{"result":{"type":"ok","cols":[],"rows":[],"affected_row_count":`+itoa(int(res.affected))+`}}}`)
			default:
				rows := ""
				for j := 0; j < res.rows; j++ {
					rows += `{"type":"row","row":[{"type":"integer","value":"1"}]}`
					if j < res.rows-1 {
						rows += ","
					}
				}
				results = append(results, `{"type":"ok","response":{"result":{"type":"ok","cols":[{"name":"1"}],"rows":[`+rows+`],"affected_row_count":0}}}`)
			}
		}
		results = append(results, `{"type":"ok","response":{"result":"close"}}`)
		resp := `{"results":[` + strings.Join(results, ",") + `]}`
		_, _ = w.Write([]byte(resp))
	})
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func newTestStore(t *testing.T, results ...stmtResult) (*TursoStore, *fakeTurso) {
	f := &fakeTurso{t: t, results: results}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	s := NewTurso(turso.New(srv.URL, "t"))
	fixed := func() time.Time { return time.Date(2026, 11, 20, 14, 0, 0, 0, time.UTC) }
	s.Now = fixed
	return s, f
}

func TestInitCreatesSchemaIdempotently(t *testing.T) {
	s, f := newTestStore(t, affectedResult(0), affectedResult(0))
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// The second Init must be memoized - no extra HTTP call.
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init (second): %v", err)
	}
	if f.calls != 1 {
		t.Fatalf("pipeline calls = %d, want 1 (memoized)", f.calls)
	}
	if len(f.gotSQLs) != 2 {
		t.Fatalf("statements = %d, want 2 (both tables)", len(f.gotSQLs))
	}
	for _, sql := range f.gotSQLs {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS") {
			t.Errorf("non-idempotent statement: %.60s", sql)
		}
	}
}

func TestInsertRowReportsInsertionAndDeDup(t *testing.T) {
	s, f := newTestStore(t, affectedResult(1), affectedResult(0))
	ctx := context.Background()
	row := Row{DeviceID: "door-1", StudentID: "s1", Status: "in", MarkedAt: "2026-11-20T08:15:00Z"}

	inserted, err := s.InsertRow(ctx, row)
	if err != nil || !inserted {
		t.Fatalf("first insert = %v, %v; want true", inserted, err)
	}
	inserted, err = s.InsertRow(ctx, row)
	if err != nil || inserted {
		t.Fatalf("retry insert = %v, %v; want false (de-duplicated)", inserted, err)
	}

	if len(f.gotArgs) != 2 || len(f.gotArgs[0]) != 5 {
		t.Fatalf("insert args = %+v, want 5 values", f.gotArgs)
	}
	want := []string{"door-1", "s1", "in", "2026-11-20T08:15:00Z", "2026-11-20T14:00:00Z"}
	for i, w := range want {
		if f.gotArgs[0][i] != w {
			t.Errorf("arg[%d] = %q, want %q", i, f.gotArgs[0][i], w)
		}
	}
	if !strings.Contains(f.gotSQLs[0], "ON CONFLICT DO NOTHING") {
		t.Errorf("insert SQL missing ON CONFLICT DO NOTHING: %.80s", f.gotSQLs[0])
	}
}

func TestUpsertDeviceAndExistenceCheck(t *testing.T) {
	s, f := newTestStore(t, affectedResult(1), rowsResult(0), rowsResult(1))
	ctx := context.Background()

	if err := s.UpsertDevice(ctx, Device{DeviceID: "door-1", Label: "front door"}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	if !strings.Contains(f.gotSQLs[0], "ON CONFLICT (device_id) DO UPDATE") {
		t.Errorf("device SQL is not an upsert: %.80s", f.gotSQLs[0])
	}
	if f.gotArgs[0][0] != "door-1" || f.gotArgs[0][1] != "front door" {
		t.Errorf("device args = %+v", f.gotArgs[0])
	}

	exists, err := s.DeviceExists(ctx, "door-9")
	if err != nil || exists {
		t.Fatalf("unknown device = %v, %v; want false", exists, err)
	}
	exists, err = s.DeviceExists(ctx, "door-1")
	if err != nil || !exists {
		t.Fatalf("known device = %v, %v; want true", exists, err)
	}
}

func TestTursoErrorSurfaces(t *testing.T) {
	s, _ := newTestStore(t, errorResult("db offline"))
	_, err := s.InsertRow(context.Background(), Row{DeviceID: "d", StudentID: "s", Status: "in", MarkedAt: "t"})
	if err == nil || !strings.Contains(err.Error(), "db offline") {
		t.Fatalf("err = %v, want db failure", err)
	}
}
