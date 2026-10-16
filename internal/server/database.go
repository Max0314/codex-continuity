package server

import (
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"regexp"
	"strconv"
	"strings"
)

// SQLite stays available for self-contained local tests. Production uses PostgreSQL.
type database struct {
	*sql.DB
	postgres bool
}
type transaction struct {
	*sql.Tx
	postgres bool
}

var caseComparison = regexp.MustCompile(`([a-zA-Z_.]+)\s*=\s*\?\s+COLLATE NOCASE`)

func postgresQuery(query string) string {
	query = caseComparison.ReplaceAllString(query, `lower(${1})=lower(?)`)
	query = strings.ReplaceAll(query, "username COLLATE NOCASE", "lower(username)")
	query = strings.ReplaceAll(query, " COLLATE NOCASE", "")
	var out strings.Builder
	single, double, index := false, false, 0
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == '\'' && !double {
			out.WriteByte(c)
			if single && i+1 < len(query) && query[i+1] == '\'' {
				out.WriteByte(query[i+1])
				i++
				continue
			}
			single = !single
			continue
		}
		if c == '"' && !single {
			double = !double
		}
		if c == '?' && !single && !double {
			index++
			out.WriteByte('$')
			out.WriteString(strconv.Itoa(index))
		} else {
			out.WriteByte(c)
		}
	}
	return out.String()
}
func sqlDialect(query string, postgres bool) string {
	if postgres {
		return postgresQuery(query)
	}
	return query
}
func (d *database) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(sqlDialect(query, d.postgres), args...)
}
func (d *database) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(sqlDialect(query, d.postgres), args...)
}
func (d *database) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(sqlDialect(query, d.postgres), args...)
}
func (d *database) Begin() (*transaction, error) {
	tx, err := d.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &transaction{tx, d.postgres}, nil
}
func (t *transaction) Exec(query string, args ...any) (sql.Result, error) {
	return t.Tx.Exec(sqlDialect(query, t.postgres), args...)
}
func (t *transaction) Query(query string, args ...any) (*sql.Rows, error) {
	return t.Tx.Query(sqlDialect(query, t.postgres), args...)
}
func (t *transaction) QueryRow(query string, args ...any) *sql.Row {
	return t.Tx.QueryRow(sqlDialect(query, t.postgres), args...)
}
