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
			{"type":"ok","response":{"result":{"type":"ok","cols":[{"name":"1"}],"rows":[{"type":"row","row":[{"type":"integer","value":"1"}]}],"affected_row_count":0}}},
			{"type":"ok","response":{"result":"close"}}
		]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "db-token")
	got, err := c.Execute(context.Background(),
		Statement{SQL: "INSERT INTO t (a, b) VALUES (?, ?)", Args: []Arg{Text("abc"), Integer(42)}},
		Statement{SQL: "SELECT 1 FROM t WHERE a = ?", Args: []Arg{Text("abc")}},
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
	// The SELECT reports zero affected rows but one returned row.
	if got[1].AffectedRowCount != 0 || got[1].RowCount != 1 {
		t.Errorf("select = %+v, want affected 0 rows 1", got[1])
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
	if args[1].Type != "integer" || string(args[1].Value) != `42` {
		t.Errorf("integer arg = %+v, want integer 42 (JSON number, not string)", args[1])
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

func TestExecuteNoStatements(t *testing.T) {
	c := New("http://unused.invalid", "t")
	got, err := c.Execute(context.Background())
	if err != nil || got != nil {
		t.Fatalf("no-op Execute = %v, %v; want nil, nil", got, err)
	}
}
