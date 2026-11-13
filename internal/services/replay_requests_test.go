package services

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestReplayTargetLogic(t *testing.T) {
	now := time.Now().UTC()
	context := ReplayContext{IP: "128.163.237.25", Hostname: "PORTAL.EXAMPLE.COM.", TenantID: "tenant"}
	rule := models.ReplayRequest{ExpiresAt: now.Add(time.Hour), Targets: []models.ReplayTarget{
		{IP: "128.163.237.0/24", TenantID: "tenant", Hostname: "portal.example.com"},
		{UserEmail: "person@example.com"},
	}}
	require.True(t, MatchReplayRequests([]models.ReplayRequest{rule}, context, now).Enabled)
	context.TenantID = "other"
	require.False(t, MatchReplayRequests([]models.ReplayRequest{rule}, context, now).Enabled)
	context.UserEmail = "person@example.com"
	require.False(t, MatchReplayRequests([]models.ReplayRequest{rule}, context, now).Enabled, "email without verified identity cannot match")
	context.UserID = "user"
	require.True(t, MatchReplayRequests([]models.ReplayRequest{rule}, context, now).Enabled, "OR group matches verified email")
	rule.ExpiresAt = now
	require.False(t, MatchReplayRequests([]models.ReplayRequest{rule}, context, now).Enabled)
	rule.ExpiresAt = now.Add(time.Hour)
	rule.RevokedAt = &now
	require.False(t, MatchReplayRequests([]models.ReplayRequest{rule}, context, now).Enabled)
	require.False(t, matchReplayTarget(models.ReplayTarget{}, context))
	require.True(t, matchReplayTarget(models.ReplayTarget{IP: "::ffff:128.163.237.25"}, context))
	require.False(t, matchReplayTarget(models.ReplayTarget{UserID: "different"}, context))
}

func TestReplayPolicyOverlap(t *testing.T) {
	now := time.Now().UTC()
	rules := []models.ReplayRequest{
		{Targets: []models.ReplayTarget{{TenantID: "tenant"}}, StartOnFailure: true, ExpiresAt: now.Add(time.Hour)},
		{Targets: []models.ReplayTarget{{TenantID: "tenant"}}, ExpiresAt: now.Add(time.Minute)},
	}
	policy := MatchReplayRequests(rules, ReplayContext{TenantID: "tenant"}, now)
	require.True(t, policy.Enabled)
	require.False(t, policy.StartOnFailure)
	require.Equal(t, rules[1].ExpiresAt, *policy.ExpiresAt)
	policy = MatchReplayRequests(rules, ReplayContext{TenantID: "tenant"}, now.Add(2*time.Minute))
	require.True(t, policy.StartOnFailure)
}

func TestReplayValidationAndCache(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/replay.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ReplayRequest{}))
	svc := NewReplayRequestService(db)
	ctx := t.Context()
	active, err := svc.Active(ctx)
	require.NoError(t, err)
	require.Empty(t, active)
	rule, err := svc.Create(ctx, models.ReplayRequest{Label: "Login problem", Targets: []models.ReplayTarget{{Hostname: " PORTAL.EXAMPLE.COM. "}}, ExpiresAt: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, rule.ID, 26)
	require.Equal(t, "portal.example.com", rule.Targets[0].Hostname)
	active, err = svc.Active(ctx)
	require.NoError(t, err)
	require.Len(t, active, 1, "create invalidates empty cache")
	require.NoError(t, svc.Revoke(ctx, rule.ID))
	active, err = svc.Active(ctx)
	require.NoError(t, err)
	require.Empty(t, active, "revoke invalidates cache")
	for _, target := range []models.ReplayTarget{{}, {IP: "bad"}, {Hostname: "https://example.com"}, {Hostname: "*.example.com"}, {UserEmail: "not an email"}} {
		_, err = svc.Create(ctx, models.ReplayRequest{Label: "Invalid", Targets: []models.ReplayTarget{target}, ExpiresAt: time.Now().Add(time.Hour)})
		require.ErrorIs(t, err, ErrReplayRequest)
	}
	_, err = svc.Create(ctx, models.ReplayRequest{Label: "Too long", Targets: []models.ReplayTarget{{TenantID: "tenant"}}, ExpiresAt: time.Now().Add(25 * time.Hour)})
	require.ErrorIs(t, err, ErrReplayRequest)
	require.NoError(t, db.Migrator().DropTable(&models.ReplayRequest{}))
	svc.loadedAt = time.Time{}
	active, err = svc.Active(ctx)
	require.Error(t, err)
	require.Nil(t, active, "database failure never returns cached eligibility")
}

func TestReplayListKeepsOlderActiveRequestsVisible(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/history.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ReplayRequest{}))
	now := time.Now().UTC()
	active := models.ReplayRequest{Label: "Still investigating", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), Targets: []models.ReplayTarget{{TenantID: "tenant"}}}
	require.NoError(t, db.Create(&active).Error)
	history := make([]models.ReplayRequest, 100)
	for i := range history {
		history[i] = models.ReplayRequest{Label: "Finished", CreatedAt: now, ExpiresAt: now.Add(-time.Minute), Targets: []models.ReplayTarget{{TenantID: "tenant"}}}
	}
	require.NoError(t, db.Create(&history).Error)
	listed, err := NewReplayRequestService(db).List(t.Context())
	require.NoError(t, err)
	require.Len(t, listed, 100)
	require.Equal(t, active.ID, listed[0].ID)
}
