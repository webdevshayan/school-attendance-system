package turso

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteSendsTypedStatementsAndReturnsAffectedCounts(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	var gotStatus = http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.WriteHeader(gotStatus)
		_, _ = w.Write([]byte(`{"results":[
			{"type":"ok","response":{"result":{"type":"ok","cols":[],"rows":[],"affected_row_count":1}}},
			{"type":"ok","response":{"result":{"type":"ok","cols":[{"name":"id"},{"name":"a"}],"rows":[[{"type":"integer","value":"7"},{"type":"text","value":"abc"}],[{"type":"integer","value":"9"},{"type":"text","value":"xyz"}]],"affected_row_count":0}}},
			{"type":"ok","response":{"result":"close"}}
		]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "db-token")
	got, err := c.Execute(context.Background(),
		Statement{SQL: "INSERT INTO t (a, b) VALUES (?, ?)", Args: []Arg{Text("abc"), Integer(42)}},
		Statement{SQL: "SELECT id, a FROM t WHERE a = ?", Args: []Arg{Text("abc")}},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("results = %v, want 2", got)
	}
	if got[0].AffectedRowCount != 1 {
		t.Errorf("insert affected = %d, want 1", got[0].AffectedRowCount)
	}
	// The SELECT reports zero affected rows and returns two rows whose cell
	// values (not just counts) must come through.
	if got[1].AffectedRowCount != 0 || got[1].RowCount != 2 || len(got[1].Rows) != 2 {
		t.Errorf("select = %+v, want affected 0 rows 2", got[1])
	}
	if got[1].Rows[0][0].Type != "integer" || got[1].Rows[0][0].Int() != 7 {
		t.Errorf("row0 col0 = %+v, want integer 7", got[1].Rows[0][0])
	}
	if got[1].Rows[1][1].Type != "text" || got[1].Rows[1][1].Str() != "xyz" {
		t.Errorf("row1 col1 = %+v, want text xyz", got[1].Rows[1][1])
	}
	if gotPath != "/v2/pipeline" {
		t.Errorf("path = %q, want /v2/pipeline", gotPath)
	}
	if gotAuth != "Bearer db-token" {
		t.Errorf("auth = %q", gotAuth)
	}

	var req struct {
		Requests []struct {
			Type string `json:"type"`
			Stmt *struct {
				SQL  string `json:"sql"`
				Args []struct {
					Type  string          `json:"type"`
					Value json.RawMessage `json:"value"`
				} `json:"args"`
			} `json:"stmt"`
		} `json:"requests"`
	}
	if err := json.Unmarshal([]byte(gotBody), &req); err != nil {
		t.Fatalf("request body not valid JSON: %v\n%s", err, gotBody)
	}
	if len(req.Requests) != 3 || req.Requests[2].Type != "close" {
		t.Fatalf("requests = %+v, want 2 executes + close", req.Requests)
	}
	args := req.Requests[0].Stmt.Args
	if len(args) != 2 {
		t.Fatalf("args = %+v", args)
	}
	if args[0].Type != "text" || string(args[0].Value) != `"abc"` {
		t.Errorf("text arg = %+v, want text \"abc\"", args[0])
	}
	if args[1].Type != "integer" || string(args[1].Value) != `"42"` {
		t.Errorf("integer arg = %+v, want integer 42 as a JSON string (Hrana expects string values)", args[1])
	}
}

func TestExecutePropagatesStatementError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"type":"error","error":{"message":"constraint failed"}}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "t")
	_, err := c.Execute(context.Background(), Statement{SQL: "INSERT INTO nope"})
	if err == nil || !strings.Contains(err.Error(), "constraint failed") {
		t.Fatalf("err = %v, want statement failure mentioning the message", err)
	}
}

func TestExecutePropagatesHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "t")
	_, err := c.Execute(context.Background(), Statement{SQL: "SELECT 1"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v, want HTTP 401 failure", err)
	}
}

// TestExecuteParsesPlainValueRows covers the plain-value array shape
// ([[11,"door-1"],…]) defensively - other libSQL deployments/versions may
// answer it; the live API uses typed cell objects (pinned by the first test).
func TestExecuteParsesPlainValueRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"type":"ok","response":{"result":{"type":"ok","cols":[{"name":"id"},{"name":"a"}],"rows":[[11,"door-1"],[12.5,"x"],[null,"y"]],"affected_row_count":0}}}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "t")
	got, err := c.Execute(context.Background(), Statement{SQL: "SELECT id, a FROM t"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	rows := got[0].Rows
	if got[0].RowCount != 3 || len(rows) != 3 {
		t.Fatalf("rows = %d (count %d), want 3", len(rows), got[0].RowCount)
	}
	if rows[0][0].Int() != 11 || rows[0][1].Str() != "door-1" {
		t.Errorf("row0 = %+v", rows[0])
	}
	if rows[1][0].Str() != "12.5" || rows[1][0].Int() != 0 {
		t.Errorf("row1 (non-integral number) = %+v; Int() is 0 by contract", rows[1][0])
	}
	if rows[2][0].Type != "null" || rows[2][0].Str() != "" {
		t.Errorf("row2 (null) = %+v", rows[2][0])
	}
}

func TestExecuteNoStatements(t *testing.T) {
	c := New("http://unused.invalid", "t")
	got, err := c.Execute(context.Background())
	if err != nil || got != nil {
		t.Fatalf("no-op Execute = %v, %v; want nil, nil", got, err)
	}
}
