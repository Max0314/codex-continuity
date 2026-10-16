package server

import "testing"

func TestPostgresPlaceholdersPreserveQuotedText(t *testing.T) {
	got := postgresQuery(`SELECT '?' AS literal FROM users WHERE username=? COLLATE NOCASE AND display_name=?`)
	want := `SELECT '?' AS literal FROM users WHERE lower(username)=lower($1) AND display_name=$2`
	if got != want {
		t.Fatalf("got %s", got)
	}
}
