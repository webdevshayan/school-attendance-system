package turso

import "testing"

// The live incident (2026-11-20): TURSO_DATABASE_URL comes in Turso's
// canonical form (libsql://...) and the pipeline client must normalize it,
// not reject it at request time with "unsupported protocol scheme".
func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"libsql://db-org.aws-ap-south-1.turso.io": "https://db-org.aws-ap-south-1.turso.io",
		"db-org.aws-ap-south-1.turso.io":          "https://db-org.aws-ap-south-1.turso.io",
		"https://db-org.turso.io/":                "https://db-org.turso.io",
		"http://localhost:8080":                   "http://localhost:8080",
		"  libsql://db-org.turso.io/  ":           "https://db-org.turso.io",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
	c := New("libsql://db-org.turso.io", "t")
	if c.BaseURL != "https://db-org.turso.io" {
		t.Errorf("New BaseURL = %q, want the https form", c.BaseURL)
	}
}
