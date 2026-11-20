// Package dbtime compares stored timestamps as instants across SQL dialects.
package dbtime

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const utcKeyLayout = "2006-01-02 15:04:05.000000000"

// Instant returns a sortable expression for a timestamp column. Column names
// (including qualifiers) and operators in this package must come from code.
func Instant(db *gorm.DB, column string) string {
	if db.Dialector.Name() != "sqlite" {
		return column
	}
	// SQLite stores timestamps as text. Normalize the whole seconds to UTC
	// and append the original fraction padded to nanoseconds. julianday()
	// would round near cutoffs and can collapse distinct instants into a tie.
	// SQLite's application formats use a space or T separator and an optional
	// Z or ±HH:MM suffix. Timestamps without an offset are interpreted as UTC.
	return fmt.Sprintf(`(strftime('%%Y-%%m-%%d %%H:%%M:%%S', substr(%[1]s, 1, 19) ||
		CASE WHEN substr(%[1]s, -6) GLOB '[+-][0-9][0-9]:[0-9][0-9]' THEN substr(%[1]s, -6) ELSE '' END)
		|| '.' || CASE WHEN substr(%[1]s, 20, 1) = '.' THEN
			substr(substr(rtrim(%[1]s, 'Z'), 21, length(rtrim(%[1]s, 'Z')) - 20 -
				CASE WHEN substr(%[1]s, -6) GLOB '[+-][0-9][0-9]:[0-9][0-9]' THEN 6 ELSE 0 END) || '000000000', 1, 9)
		ELSE '000000000' END)`, column)
}

// Compare builds a timestamp predicate with a bound cutoff. Use it directly in
// Where, or as a value for ? when embedding it in a larger SQL expression.
func Compare(db *gorm.DB, column, operator string, cutoff time.Time) clause.Expr {
	var value any = cutoff
	if db.Dialector.Name() == "sqlite" {
		value = cutoff.UTC().Format(utcKeyLayout)
	}
	return clause.Expr{SQL: Instant(db, column) + " " + operator + " ?", Vars: []any{value}}
}
