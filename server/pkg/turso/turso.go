// Package turso is a minimal libSQL-over-HTTP client (the v2 pipeline API)
// built on the standard library only. Turso's own Go SDK embeds a native
// libsql core, which Vercel's serverless Go runtime cannot run; the HTTP API
// needs nothing beyond net/http.
package turso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Arg is one positional statement argument.
type Arg struct {
	Type  string `json:"type"`  // "text" or "integer"
	Value any    `json:"value"` // string for text, int64 for integer
}

// Text builds a text argument.
func Text(v string) Arg { return Arg{Type: "text", Value: v} }

// Integer builds an integer argument.
func Integer(v int64) Arg { return Arg{Type: "integer", Value: v} }

// Statement is one SQL statement with positional arguments.
type Statement struct {
	SQL  string
	Args []Arg
}

// Client talks to a Turso database over the v2 pipeline API.
type Client struct {
	BaseURL string // e.g. https://db-name-org.turso.io (no trailing slash)
	Token   string
	HTTP    *http.Client
}

// New builds a client; the default HTTP client has a conservative timeout so
// a slow database cannot pin a serverless function until its limit.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Cell is one typed value in a returned row (libSQL wire types: text,
// integer, real, blob, null). The helpers coerce the common cases.
type Cell struct {
	Type  string `json:"type"`
	Value string `json:"value"` // libSQL encodes every value as a string
}

// Str returns the cell value as a string (works for every type).
func (c Cell) Str() string { return c.Value }

// Int parses the cell as an integer; 0 for null/unparseable values.
func (c Cell) Int() int64 {
	n, _ := strconv.ParseInt(c.Value, 10, 64)
	return n
}

// Result summarizes one executed statement. SELECTs surface their rows.
type Result struct {
	AffectedRowCount int64    // DML statements (INSERT/UPDATE/DELETE)
	RowCount         int      // len(Rows)
	Rows             [][]Cell // SELECT statements: the returned rows, in order
}

// Execute runs the statements in one pipeline request and returns a Result
// per statement, in order. It fails fast on transport, HTTP, or
// per-statement errors.
func (c *Client) Execute(ctx context.Context, stmts ...Statement) ([]Result, error) {
	if len(stmts) == 0 {
		return nil, nil
	}
	requests := make([]map[string]any, 0, len(stmts)+1)
	for _, s := range stmts {
		args := make([]Arg, 0, len(s.Args))
		args = append(args, s.Args...)
		requests = append(requests, map[string]any{
			"type": "execute",
			"stmt": map[string]any{"sql": s.SQL, "args": args},
		})
	}
	requests = append(requests, map[string]any{"type": "close"})
	payload, err := json.Marshal(map[string]any{"requests": requests})
	if err != nil {
		return nil, fmt.Errorf("turso: encode pipeline request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v2/pipeline", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("turso: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("turso: pipeline request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("turso: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("turso: pipeline HTTP %d: %.200s", resp.StatusCode, body)
	}

	var parsed struct {
		Results []struct {
			Type  string `json:"type"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Response *struct {
				Result json.RawMessage `json:"result"`
			} `json:"response"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("turso: decode pipeline response: %w", err)
	}

	affected := make([]Result, 0, len(stmts))
	for i, res := range parsed.Results {
		if i >= len(stmts) {
			break // the trailing "close" result
		}
		if res.Type != "ok" || res.Response == nil {
			msg := "unknown pipeline error"
			if res.Error != nil {
				msg = res.Error.Message
			}
			return nil, fmt.Errorf("turso: statement %d failed: %s", i, msg)
		}
		var exec struct {
			AffectedRowCount int64           `json:"affected_row_count"`
			Rows             json.RawMessage `json:"rows"`
		}
		if err := json.Unmarshal(res.Response.Result, &exec); err != nil {
			// The "close" response carries a string result; only execute
			// results carry the row/affected object, so a decode failure here
			// means the server answered out of order.
			return nil, fmt.Errorf("turso: statement %d: unexpected result shape", i)
		}
		// Row objects: {"type":"row","row":[{"type":"text","value":"x"},…]}.
		// Absent/null rows field (DML) → nil.
		var rowObjs []struct {
			Row []Cell `json:"row"`
		}
		_ = json.Unmarshal(exec.Rows, &rowObjs)
		rows := make([][]Cell, 0, len(rowObjs))
		for _, obj := range rowObjs {
			rows = append(rows, obj.Row)
		}
		affected = append(affected, Result{
			AffectedRowCount: exec.AffectedRowCount,
			RowCount:         len(rows),
			Rows:             rows,
		})
	}
	if len(affected) != len(stmts) {
		return nil, fmt.Errorf("turso: pipeline returned %d results for %d statements", len(parsed.Results), len(stmts))
	}
	return affected, nil
}
