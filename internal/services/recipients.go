package services

import (
	"context"
	"sort"
	"time"

	"github.com/pixelcop/clientshare/internal/dbtime"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecipientContact is deliberately not a User: directory lookup never returns
// authentication, roles, credentials, or associations outside the actor's scope.
type RecipientContact struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Email           string            `json:"email"`
	Clients         []RecipientClient `json:"clients" gorm:"-"`
	CurrentClient   bool              `json:"current_client" gorm:"-"`
	WorkspaceAccess bool              `json:"workspace_access" gorm:"-"`
}

type RecipientClient struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RecipientService struct{ DB *gorm.DB }

func recipientAssociations(db *gorm.DB, tenantID string) *gorm.DB {
	now := time.Now()
	active := dbtime.Compare(db, "expires_at", ">", now)
	assignmentActive := dbtime.Compare(db, "ra.expires_at", ">", now)
	return db.Raw(`
		SELECT user_id, client_id FROM user_clients WHERE tenant_id = ?
		UNION SELECT user_id, client_id FROM item_grants WHERE tenant_id = ? AND user_id IS NOT NULL AND revoked_at IS NULL AND (expires_at IS NULL OR ?)
		UNION SELECT recipient_user_id AS user_id, client_id FROM access_invitations WHERE tenant_id = ? AND recipient_user_id IS NOT NULL AND accepted_at IS NULL AND revoked_at IS NULL AND ?
		UNION SELECT ra.user_id, ra.client_id FROM role_assignments ra WHERE ra.tenant_id = ? AND ra.user_id IS NOT NULL AND ra.revoked_at IS NULL AND (ra.expires_at IS NULL OR ?)
		UNION SELECT gm.user_id, ra.client_id FROM role_assignments ra JOIN access_group_members gm ON gm.tenant_id = ra.tenant_id AND gm.group_id = ra.group_id WHERE ra.tenant_id = ? AND ra.revoked_at IS NULL AND (ra.expires_at IS NULL OR ?)`,
		tenantID, tenantID, active, tenantID, active, tenantID, assignmentActive, tenantID, assignmentActive)
}

// Search reuses identities and existing relationships without mutating either.
// Only staff with whole-workspace visibility may use the directory. Sharing as
// a client or an anonymous recipient must not turn into directory discovery.
func (s *RecipientService) Search(ctx context.Context, req AccessRequest, search string, across bool) ([]RecipientContact, error) {
	return s.search(ctx, req, search, across, false)
}

func (s *RecipientService) SearchWorkspaceClients(ctx context.Context, req AccessRequest, search string) ([]RecipientContact, error) {
	return s.search(ctx, req, search, false, true)
}

func (s *RecipientService) search(ctx context.Context, req AccessRequest, search string, across, workspaceClientsOnly bool) ([]RecipientContact, error) {
	contacts := []RecipientContact{}
	if req.Actor.UserID == "" || req.Actor.LinkID != "" || req.TenantID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	db := s.DB.WithContext(ctx)
	var actor models.User
	if err := db.Where("tenant_id = ? AND id = ?", req.TenantID, req.Actor.UserID).First(&actor).Error; err != nil {
		return nil, err
	}
	if actor.Role != "admin" && actor.Role != "manager" {
		return nil, gorm.ErrRecordNotFound
	}
	scope, err := (&AccessService{DB: db}).ClientScope(ctx, req.TenantID, actor.ID, AccessContext)
	if err != nil {
		return nil, err
	}
	if req.ClientID != "" {
		var count int64
		if err := scope.Session(&gorm.Session{}).Where("clients.id = ?", req.ClientID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, gorm.ErrRecordNotFound
		}
	} else if !across {
		return contacts, nil
	}
	var clients []models.Client
	if err := scope.Find(&clients).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(clients))
	clientNames := map[string]string{}
	for _, client := range clients {
		ids = append(ids, client.ID)
		clientNames[client.ID] = client.Name
	}
	if len(ids) == 0 {
		return contacts, nil
	}
	// One row per identity/client association. Pending invitations and item-only
	// access are contacts, but are never presented as workspace membership.
	associations := recipientAssociations(db, req.TenantID)
	// GORM embeds the union as a subquery so all filtering stays tenant-scoped
	// and the result limit applies to identities, not association rows.
	q := db.Table("users").Select("users.id, users.name, users.email").
		Where("users.tenant_id = ?", req.TenantID)
	if workspaceClientsOnly {
		q = q.Where("users.role IN ?", []string{"client", "customer"})
	}
	visible := db.Table("(?) AS associations", associations).Select("user_id").Where("client_id IN ?", ids)
	current := db.Table("(?) AS associations", associations).Select("user_id").Where("client_id = ?", req.ClientID)
	if across {
		if actor.Role != "admin" {
			q = q.Where("users.id IN (?)", visible)
		}
	} else {
		q = q.Where("users.id IN (?)", current)
	}
	query := normalizePeopleSearch(search)
	q = q.Order(clause.OrderBy{Expression: gorm.Expr("CASE WHEN users.id IN (?) THEN 0 ELSE 1 END, LOWER(users.name), LOWER(users.email), users.id", current)})
	if query == "" {
		q = q.Limit(20)
	}
	if err := q.Find(&contacts).Error; err != nil {
		return nil, err
	}
	if query != "" {
		ranks := make(map[string]int)
		matches := contacts[:0]
		for _, contact := range contacts {
			if rank := peopleMatch(query, contact.Name, contact.Email); rank < noPeopleMatch {
				ranks[contact.ID] = rank
				matches = append(matches, contact)
			}
		}
		sort.SliceStable(matches, func(i, j int) bool { return ranks[matches[i].ID] < ranks[matches[j].ID] })
		contacts = matches[:min(len(matches), 20)]
	}
	userIDs := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		userIDs = append(userIDs, contact.ID)
	}
	if len(userIDs) == 0 {
		return contacts, nil
	}
	var rows []struct{ UserID, ClientID string }
	if err := db.Table("(?) AS associations", associations).Where("client_id IN ? AND user_id IN ?", ids, userIDs).Order("client_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	byUser := map[string][]RecipientClient{}
	for _, row := range rows {
		byUser[row.UserID] = append(byUser[row.UserID], RecipientClient{ID: row.ClientID, Name: clientNames[row.ClientID]})
	}
	for i := range contacts {
		contacts[i].Clients = byUser[contacts[i].ID]
		if contacts[i].Clients == nil {
			contacts[i].Clients = []RecipientClient{}
		}
		for _, client := range contacts[i].Clients {
			if client.ID == req.ClientID {
				contacts[i].CurrentClient = true
			}
		}
		if req.ClientID != "" {
			decision, err := (&AccessService{DB: db}).Resolve(ctx, AccessRequest{
				TenantID: req.TenantID, ClientID: req.ClientID, Actor: AccessActor{UserID: contacts[i].ID}, Operation: AccessContext,
			})
			if err != nil {
				return nil, err
			}
			contacts[i].WorkspaceAccess = decision.Allowed
		}
	}
	if workspaceClientsOnly {
		allowed := contacts[:0]
		for _, contact := range contacts {
			if contact.WorkspaceAccess {
				allowed = append(allowed, contact)
			}
		}
		return allowed, nil
	}
	return contacts, nil
}
