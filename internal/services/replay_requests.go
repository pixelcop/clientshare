package services

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const replayCacheTTL = 5 * time.Second
const maxActiveReplayRequests = 100

var ErrReplayRequest = errors.New("invalid replay request")

type ReplayContext struct{ IP, Hostname, TenantID, UserID, UserEmail string }

// ReplayPolicy is intentionally free of targeting criteria and user identifiers.
type ReplayPolicy struct {
	Enabled        bool       `json:"enabled"`
	StartOnFailure bool       `json:"start_on_failure"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type ReplayRequestService struct {
	db       *gorm.DB
	mu       sync.Mutex
	loadedAt time.Time
	rules    []models.ReplayRequest
	loadErr  error
}

func NewReplayRequestService(db *gorm.DB) *ReplayRequestService { return &ReplayRequestService{db: db} }

func (s *ReplayRequestService) List(ctx context.Context) ([]models.ReplayRequest, error) {
	rules := []models.ReplayRequest{}
	// Active requests must remain visible/revocable even when newer requests
	// have already expired. Fill the remaining space with recent history.
	err := s.db.WithContext(ctx).Clauses(clause.OrderBy{Expression: clause.Expr{
		SQL:  "CASE WHEN revoked_at IS NULL AND expires_at > ? THEN 0 ELSE 1 END, created_at DESC",
		Vars: []any{time.Now().UTC()},
	}}).Limit(100).Find(&rules).Error
	return rules, err
}

func (s *ReplayRequestService) Create(ctx context.Context, rule models.ReplayRequest) (*models.ReplayRequest, error) {
	now := time.Now().UTC()
	if err := validateReplayRequest(&rule, now); err != nil {
		return nil, err
	}
	if err := s.validateTargetIDs(ctx, rule.Targets); err != nil {
		return nil, err
	}
	// Serialize mutations with local cache loads. Other replicas refresh within 5s.
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.ReplayRequest{}).Where("revoked_at IS NULL AND expires_at > ?", now).Count(&count).Error; err != nil {
			return err
		}
		if count >= maxActiveReplayRequests {
			return fmt.Errorf("%w: at most 100 active requests", ErrReplayRequest)
		}
		return tx.Create(&rule).Error
	})
	s.loadedAt = time.Time{}
	return &rule, err
}

func (s *ReplayRequestService) Revoke(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.db.WithContext(ctx).Model(&models.ReplayRequest{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", time.Now().UTC())
	s.loadedAt = time.Time{}
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Active is bounded and cached across requests, including the usual empty case.
// A failed refresh discards stale rules rather than extending collection.
func (s *ReplayRequestService) Active(ctx context.Context) ([]models.ReplayRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if s.loadedAt.IsZero() || now.Sub(s.loadedAt) >= replayCacheTTL {
		var rules []models.ReplayRequest
		err := s.db.WithContext(ctx).Where("revoked_at IS NULL AND expires_at > ?", now).Limit(maxActiveReplayRequests + 1).Find(&rules).Error
		if err != nil || len(rules) > maxActiveReplayRequests {
			s.rules = nil
			if err == nil {
				err = errors.New("replay request limit exceeded")
			}
			s.loadedAt, s.loadErr = now, err
			return nil, err
		}
		s.rules, s.loadedAt = rules, now
		s.loadErr = nil
	}
	return s.rules, s.loadErr
}

func MatchReplayRequests(rules []models.ReplayRequest, c ReplayContext, now time.Time) ReplayPolicy {
	policy := ReplayPolicy{StartOnFailure: true}
	for _, rule := range rules {
		if rule.RevokedAt != nil || !rule.ExpiresAt.After(now) {
			continue
		}
		for _, target := range rule.Targets {
			if !matchReplayTarget(target, c) {
				continue
			}
			policy.Enabled = true
			policy.StartOnFailure = policy.StartOnFailure && rule.StartOnFailure
			// Re-evaluate at the earliest expiry, so overlapping immediate and
			// failure-only requests cannot extend each other's recording mode.
			if policy.ExpiresAt == nil || rule.ExpiresAt.Before(*policy.ExpiresAt) {
				expires := rule.ExpiresAt
				policy.ExpiresAt = &expires
			}
			break
		}
	}
	if !policy.Enabled {
		policy.StartOnFailure = false
	}
	return policy
}

func matchReplayTarget(t models.ReplayTarget, c ReplayContext) bool {
	if t == (models.ReplayTarget{}) {
		return false
	}
	if t.IP != "" {
		addr, err := netip.ParseAddr(c.IP)
		if err != nil {
			return false
		}
		if prefix, err := netip.ParsePrefix(t.IP); err == nil {
			if !prefix.Contains(addr.Unmap()) {
				return false
			}
		} else if wanted, err := netip.ParseAddr(t.IP); err != nil || wanted.Unmap() != addr.Unmap() {
			return false
		}
	}
	return (t.Hostname == "" || t.Hostname == strings.TrimSuffix(strings.ToLower(c.Hostname), ".")) &&
		(t.TenantID == "" || t.TenantID == c.TenantID) &&
		(t.UserID == "" || (c.UserID != "" && t.UserID == c.UserID)) &&
		(t.UserEmail == "" || (c.UserID != "" && strings.EqualFold(t.UserEmail, c.UserEmail)))
}

func validateReplayRequest(r *models.ReplayRequest, now time.Time) error {
	r.ID, r.RevokedAt, r.CreatedAt = "", nil, now
	r.ExpiresAt = r.ExpiresAt.UTC()
	r.Label = strings.TrimSpace(r.Label)
	invalid := func(message string) error { return fmt.Errorf("%w: %s", ErrReplayRequest, message) }
	if len(r.Label) == 0 || len(r.Label) > 120 {
		return invalid("label must be 1–120 characters")
	}
	if !r.ExpiresAt.After(now) || r.ExpiresAt.After(now.Add(24*time.Hour)) {
		return invalid("expiry must be in the next 24 hours")
	}
	if len(r.Targets) == 0 || len(r.Targets) > 10 {
		return invalid("provide 1–10 targeting groups")
	}
	for i := range r.Targets {
		t := &r.Targets[i]
		t.IP, t.Hostname, t.TenantID, t.UserID, t.UserEmail = strings.TrimSpace(t.IP), strings.TrimSuffix(strings.ToLower(strings.TrimSpace(t.Hostname)), "."), strings.TrimSpace(t.TenantID), strings.TrimSpace(t.UserID), strings.ToLower(strings.TrimSpace(t.UserEmail))
		if *t == (models.ReplayTarget{}) {
			return invalid("targeting groups cannot be empty")
		}
		if len(t.TenantID) > 26 || len(t.UserID) > 26 {
			return invalid("IDs must be at most 26 characters")
		}
		if t.IP != "" {
			if addr, err := netip.ParseAddr(t.IP); err == nil {
				t.IP = addr.Unmap().String()
			} else if prefix, err := netip.ParsePrefix(t.IP); err == nil {
				t.IP = prefix.Masked().String()
			} else {
				return invalid("IP must be an address or CIDR")
			}
		}
		if t.Hostname != "" {
			if len(t.Hostname) > 253 || strings.ContainsAny(t.Hostname, "/: *?#@") {
				return invalid("hostname must be an exact DNS hostname without scheme or port")
			}
			for _, label := range strings.Split(t.Hostname, ".") {
				if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
					return invalid("invalid hostname")
				}
				for _, ch := range label {
					if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
						return invalid("invalid hostname")
					}
				}
			}
		}
		if t.UserEmail != "" {
			addr, err := mail.ParseAddress(t.UserEmail)
			if err != nil || addr.Address != t.UserEmail || len(t.UserEmail) > 254 {
				return invalid("invalid user email")
			}
		}
	}
	return nil
}
