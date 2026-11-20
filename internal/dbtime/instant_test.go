package dbtime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgresUsesNativeTimestampComparison(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=localhost user=postgres dbname=postgres"), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true,
	})
	require.NoError(t, err)
	cutoff := time.Date(2026, 11, 1, 1, 15, 0, 123456000, time.FixedZone("EST", -5*3600))
	var rows []struct{ ID string }
	stmt := db.Table("sessions").Where(Compare(db, "expires_at", ">", cutoff)).
		Order(Instant(db, "expires_at") + " ASC").Find(&rows).Statement
	require.Equal(t, `SELECT * FROM "sessions" WHERE expires_at > $1 ORDER BY expires_at ASC`, stmt.SQL.String())
	require.Equal(t, []any{cutoff}, stmt.Vars)
}
