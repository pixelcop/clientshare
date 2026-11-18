package services

import (
	"context"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type CommentMention struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Email               string `json:"email,omitempty"`
	PreviouslyMentioned bool   `json:"previously_mentioned,omitempty"`
}

// mentionQuery never makes file access into a tenant directory permission.
// Staff with whole-client read authority can use that client's contacts; other
// readers can mention themselves and people already visible in the conversation.
func (s *FileCommentService) mentionQuery(ctx context.Context, req *AccessRequest, actor models.User, visibility string) (*gorm.DB, bool, error) {
	if visibility != CommentShared && visibility != CommentTeam {
		return nil, false, ErrCommentInput
	}
	staff := actor.Role == "admin" || actor.Role == "manager"
	if visibility == CommentTeam && !staff {
		return nil, false, gorm.ErrRecordNotFound
	}
	db := s.DB.WithContext(ctx)
	clientID := req.ClientID
	if clientID == "" {
		var file models.File
		if err := db.Select("client_id").Where("tenant_id = ? AND id = ?", req.TenantID, req.FileID).First(&file).Error; err != nil {
			return nil, false, err
		}
		clientID = file.ClientID
		req.ClientID = clientID
	}
	directory := false
	if staff {
		decision, err := (&AccessService{DB: db}).Resolve(ctx, AccessRequest{
			TenantID: req.TenantID, ClientID: clientID, Actor: req.Actor, Operation: AccessRead, Now: req.Now,
		})
		if err != nil {
			return nil, false, err
		}
		directory = decision.Allowed
	}
	q := db.Model(&models.User{}).Where("users.tenant_id = ?", req.TenantID)
	if directory {
		q = q.Where(`(users.role = 'admin' OR (users.role = 'manager' AND users.manager_full_access = TRUE)
			OR EXISTS (SELECT 1 FROM user_clients uc WHERE uc.tenant_id=users.tenant_id AND uc.user_id=users.id AND uc.client_id=?)
			OR EXISTS (SELECT 1 FROM item_grants g WHERE g.tenant_id=users.tenant_id AND g.user_id=users.id AND g.client_id=?)
			OR EXISTS (SELECT 1 FROM role_assignments ra WHERE ra.tenant_id=users.tenant_id AND ra.client_id=? AND
				(ra.user_id=users.id OR EXISTS (SELECT 1 FROM access_group_members gm WHERE gm.tenant_id=ra.tenant_id AND gm.group_id=ra.group_id AND gm.user_id=users.id))))`,
			clientID, clientID, clientID)
	} else {
		authors := commentQuery(db, *req, visibility == CommentTeam).Select("author_id")
		q = q.Where("(users.id = ? OR users.id IN (?))", actor.ID, authors)
	}
	if visibility == CommentTeam {
		q = q.Where("users.role IN ?", []string{"admin", "manager"})
	}
	return q, directory, nil
}

func (s *FileCommentService) mentionCanRead(ctx context.Context, req AccessRequest, userID string) (bool, error) {
	req.Actor, req.Operation = AccessActor{UserID: userID}, AccessRead
	decision, err := (&AccessService{DB: s.DB}).Resolve(ctx, req)
	return decision.Allowed, err
}

func (s *FileCommentService) Mentions(ctx context.Context, req AccessRequest, search, visibility string) ([]CommentMention, error) {
	actor, _, err := s.authorize(ctx, req)
	if err != nil {
		return nil, err
	}
	q, directory, err := s.mentionQuery(ctx, &req, actor, visibility)
	if err != nil {
		return nil, err
	}
	result := []CommentMention{}
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > 200 {
		return nil, ErrCommentInput
	}
	query := normalizePeopleSearch(search)
	var bodies []string
	if err := commentQuery(s.DB.WithContext(ctx), req, visibility == CommentTeam).
		Where("deleted_at IS NULL AND body LIKE ?", "%mention:%").Pluck("body", &bodies).Error; err != nil {
		return nil, err
	}
	mentioned := make(map[string]bool)
	for _, body := range bodies {
		_, ids, err := RenderCommentMarkdown(body)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			mentioned[id] = true
		}
	}
	var users []struct{ ID, Name, Email string }
	if err := q.Select("users.id", "users.name", "users.email").Find(&users).Error; err != nil {
		return nil, err
	}
	type match struct {
		person CommentMention
		rank   int
		name   string
	}
	var matches []match
	for _, user := range users {
		name := strings.TrimSpace(user.Name)
		if name == "" {
			name = "Portal user"
		}
		mention := CommentMention{ID: user.ID, Name: name, PreviouslyMentioned: mentioned[user.ID]}
		if directory {
			mention.Email = user.Email
		}
		if rank := peopleMatch(query, name, mention.Email); rank < noPeopleMatch {
			matches = append(matches, match{mention, rank, normalizePeopleSearch(name)})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.person.PreviouslyMentioned != b.person.PreviouslyMentioned {
			return a.person.PreviouslyMentioned
		}
		if a.name != b.name {
			return a.name < b.name
		}
		return a.person.ID < b.person.ID
	})
	for _, match := range matches {
		allowed, err := s.mentionCanRead(ctx, req, match.person.ID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		result = append(result, match.person)
		if len(result) == 8 {
			break
		}
	}
	return result, nil
}

func (s *FileCommentService) validateMentions(ctx context.Context, req AccessRequest, actor models.User, visibility, body string) error {
	_, ids, err := RenderCommentMarkdown(body)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 20 {
		return ErrCommentMention
	}
	q, _, err := s.mentionQuery(ctx, &req, actor, visibility)
	if err != nil {
		return err
	}
	var users []models.User
	if err := q.Where("users.id IN ?", ids).Find(&users).Error; err != nil {
		return err
	}
	if len(users) != len(ids) {
		return ErrCommentMention
	}
	for _, user := range users {
		allowed, err := s.mentionCanRead(ctx, req, user.ID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrCommentMention
		}
	}
	return nil
}

func (s *FileCommentService) Preview(ctx context.Context, req AccessRequest, input CommentInput) (string, error) {
	actor, team, err := s.authorize(ctx, req)
	if err != nil {
		return "", err
	}
	if !validCommentBody(input.Body) || (input.Visibility != CommentShared && input.Visibility != CommentTeam) {
		return "", ErrCommentInput
	}
	if input.Visibility == CommentTeam && !team {
		return "", gorm.ErrRecordNotFound
	}
	if err := s.validateMentions(ctx, req, actor, input.Visibility, input.Body); err != nil {
		return "", err
	}
	rendered, _, err := RenderCommentMarkdown(input.Body)
	return rendered, err
}
