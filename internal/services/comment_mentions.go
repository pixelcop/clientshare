package services

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type CommentMention struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// mentionQuery never makes file access into a tenant directory permission.
// Staff with whole-client read authority can use that client's contacts; other
// readers can mention themselves and people already visible in the conversation.
func (s *FileCommentService) mentionQuery(ctx context.Context, req AccessRequest, actor models.User, visibility string) (*gorm.DB, bool, error) {
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
		authors := commentQuery(db, req, visibility == CommentTeam).Select("author_id")
		q = q.Where("(users.id = ? OR users.id IN (?))", actor.ID, authors)
	}
	if visibility == CommentTeam {
		q = q.Where("users.role IN ?", []string{"admin", "manager"})
	}
	return q, directory, nil
}

func (s *FileCommentService) mentionCanRead(ctx context.Context, req AccessRequest, user models.User) (bool, error) {
	var file models.File
	if err := s.DB.WithContext(ctx).Select("client_id").Where("tenant_id = ? AND id = ?", req.TenantID, req.FileID).First(&file).Error; err != nil {
		return false, err
	}
	req.ClientID, req.Actor, req.Operation = file.ClientID, AccessActor{UserID: user.ID}, AccessRead
	decision, err := (&AccessService{DB: s.DB}).Resolve(ctx, req)
	return decision.Allowed, err
}

func (s *FileCommentService) Mentions(ctx context.Context, req AccessRequest, search, visibility string) ([]CommentMention, error) {
	actor, _, err := s.authorize(ctx, req)
	if err != nil {
		return nil, err
	}
	q, directory, err := s.mentionQuery(ctx, req, actor, visibility)
	if err != nil {
		return nil, err
	}
	result := []CommentMention{}
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > 200 {
		return nil, ErrCommentInput
	}
	search = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(search))
	like := "%" + search + "%"
	if directory {
		q = q.Where("(LOWER(users.name) LIKE ? ESCAPE '!' OR LOWER(users.email) LIKE ? ESCAPE '!')", like, like)
	} else {
		q = q.Where("LOWER(users.name) LIKE ? ESCAPE '!'", like)
	}
	var users []models.User
	// Bound permission work as well as response size. More specific text can
	// narrow a crowded name match; this is an autocomplete, not a full roster.
	if err := q.Order("LOWER(users.name), users.id").Limit(40).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		allowed, err := s.mentionCanRead(ctx, req, user)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}
		name := strings.TrimSpace(user.Name)
		if name == "" {
			name = "Portal user"
		}
		mention := CommentMention{ID: user.ID, Name: name}
		if directory {
			mention.Email = user.Email
		}
		result = append(result, mention)
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
	q, _, err := s.mentionQuery(ctx, req, actor, visibility)
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
		allowed, err := s.mentionCanRead(ctx, req, user)
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
