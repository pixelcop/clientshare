package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CommentShared    = "shared"
	CommentTeam      = "team"
	MaxCommentLength = 4000
	commentPageSize  = 50
)

var (
	ErrCommentInput    = errors.New("invalid comment")
	ErrCommentConflict = errors.New("comment changed; reload before saving")
	ErrCommentMention  = errors.New("unavailable comment mention")
)

type FileCommentService struct {
	DB                  *gorm.DB
	NotificationBaseURL string
}

type FileCommentPage struct {
	Items            []models.FileComment `json:"items"`
	NextCursor       string               `json:"next_cursor"`
	CanInternal      bool                 `json:"can_internal"`
	TotalCount       int64                `json:"total_count"`
	FocusUnavailable bool                 `json:"focus_unavailable,omitempty"`
	FocusHasNewer    bool                 `json:"focus_has_newer,omitempty"`
}

// Count includes only live comments visible to the actor, not removed
// placeholders or private team notes the actor cannot read.
func (s *FileCommentService) Count(ctx context.Context, req AccessRequest) (int64, error) {
	_, team, err := s.authorize(ctx, req)
	if err != nil {
		return 0, err
	}
	return visibleCommentCount(s.DB.WithContext(ctx), req, team)
}

func visibleCommentCount(db *gorm.DB, req AccessRequest, team bool) (int64, error) {
	var count int64
	err := commentQuery(db, req, team).Where("deleted_at IS NULL").Count(&count).Error
	return count, err
}

type CommentInput struct {
	Body       string `json:"body"`
	Visibility string `json:"visibility"`
}

// authorize reloads both identity and exact-file authority. Link possession
// (even alongside a staff identity) is never permission to read conversations.
func (s *FileCommentService) authorize(ctx context.Context, req AccessRequest) (models.User, bool, error) {
	var user models.User
	if req.TenantID == "" || req.Actor.UserID == "" || req.Actor.LinkID != "" || req.FileID == "" {
		return user, false, gorm.ErrRecordNotFound
	}
	db := s.DB.WithContext(ctx)
	if err := db.Where("tenant_id = ? AND id = ?", req.TenantID, req.Actor.UserID).First(&user).Error; err != nil {
		return user, false, err
	}
	var file models.File
	if err := db.Where("tenant_id = ? AND id = ? AND type = ? AND deleted_at IS NULL", req.TenantID, req.FileID, "file").First(&file).Error; err != nil {
		return user, false, err
	}
	if req.ClientID != "" && req.ClientID != file.ClientID {
		return user, false, gorm.ErrRecordNotFound
	}
	req.ClientID, req.Operation = file.ClientID, AccessRead
	decision, err := (&AccessService{DB: db}).Resolve(ctx, req)
	if err != nil {
		return user, false, err
	}
	if !decision.Allowed {
		return user, false, gorm.ErrRecordNotFound
	}
	return user, user.Role == "admin" || user.Role == "manager", nil
}

func commentQuery(db *gorm.DB, req AccessRequest, team bool) *gorm.DB {
	q := db.Model(&models.FileComment{}).Where("tenant_id = ? AND file_id = ?", req.TenantID, req.FileID)
	if !team {
		q = q.Where("visibility = ?", CommentShared)
	}
	return q
}

func (s *FileCommentService) List(ctx context.Context, req AccessRequest, before string) (*FileCommentPage, error) {
	return s.ListAround(ctx, req, before, "")
}

// ListAround reads a bounded window of visible comments, using the same
// audience filter for the anchor and its neighbors.
func (s *FileCommentService) ListAround(ctx context.Context, req AccessRequest, before, focus string) (*FileCommentPage, error) {
	actor, team, err := s.authorize(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(before) > 26 || len(focus) > 26 || (before != "" && focus != "") {
		return nil, ErrCommentInput
	}
	db := s.DB.WithContext(ctx)
	base := func() *gorm.DB { return commentQuery(db, req, team) }
	result := &FileCommentPage{Items: []models.FileComment{}, CanInternal: team}
	result.TotalCount, err = visibleCommentCount(db, req, team)
	if err != nil {
		return nil, err
	}
	if focus != "" {
		var anchor models.FileComment
		err := base().Where("id = ? AND deleted_at IS NULL", focus).First(&anchor).Error
		if err == nil {
			const newerLimit = 15
			var newer []models.FileComment
			if err := base().Where("id > ?", anchor.ID).Order("id ASC").Limit(newerLimit + 1).Find(&newer).Error; err != nil {
				return nil, err
			}
			if len(newer) > newerLimit {
				result.FocusHasNewer = true
				newer = newer[:newerLimit]
			}
			for i := len(newer) - 1; i >= 0; i-- {
				result.Items = append(result.Items, newer[i])
			}
			before = anchor.ID
			result.Items = append(result.Items, anchor)
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			result.FocusUnavailable = true
		} else {
			return nil, err
		}
	}
	remaining := commentPageSize - len(result.Items)
	q := base()
	if before != "" {
		q = q.Where("id < ?", before)
	}
	var older []models.FileComment
	if err := q.Order("id DESC").Limit(remaining + 1).Find(&older).Error; err != nil {
		return nil, err
	}
	if len(older) > remaining {
		older = older[:remaining]
		result.NextCursor = older[len(older)-1].ID
	}
	result.Items = append(result.Items, older...)
	if err := decorateComments(db, req, actor, result.Items); err != nil {
		return nil, err
	}
	return result, nil
}

func decorateComments(db *gorm.DB, req AccessRequest, actor models.User, comments []models.FileComment) error {
	if len(comments) == 0 {
		return nil
	}
	ids := make([]string, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.AuthorID)
	}
	var users []models.User
	if err := db.Select("id, name").Where("tenant_id = ? AND id IN ?", actor.TenantID, ids).Find(&users).Error; err != nil {
		return err
	}
	names := map[string]string{}
	for _, user := range users {
		names[user.ID] = strings.TrimSpace(user.Name)
	}
	renderers := make([]func(map[string]string) (string, error), len(comments))
	byAudience := map[string]map[string]bool{}
	for i := range comments {
		c := &comments[i]
		c.AuthorName = names[c.AuthorID]
		if c.AuthorName == "" {
			c.AuthorName = "Portal user"
		}
		c.CanViewHistory = c.AuthorID == actor.ID
		c.CanEdit = c.CanViewHistory && c.DeletedAt == nil
		if c.DeletedAt == nil {
			render, ids := prepareCommentMarkdown(c.Body)
			renderers[i] = render
			if len(ids) != 0 {
				if byAudience[c.Visibility] == nil {
					byAudience[c.Visibility] = map[string]bool{}
				}
				for _, id := range ids {
					byAudience[c.Visibility][id] = true
				}
			}
		}
	}
	live := map[string]map[string]string{}
	for audience, ids := range byAudience {
		keys := make([]string, 0, len(ids))
		for id := range ids {
			keys = append(keys, id)
		}
		q, _, err := (&FileCommentService{DB: db}).mentionQuery(db.Statement.Context, &req, actor, audience)
		if err != nil {
			return err
		}
		var people []struct{ ID, Name string }
		if err := q.Select("users.id", "users.name").Where("users.id IN ?", keys).Find(&people).Error; err != nil {
			return err
		}
		live[audience] = make(map[string]string, len(people))
		for _, person := range people {
			live[audience][person.ID] = strings.TrimSpace(person.Name)
		}
	}
	for i, render := range renderers {
		if render != nil {
			c := &comments[i]
			rendered, err := render(live[c.Visibility])
			if err != nil {
				return err
			}
			c.BodyHTML = &rendered
		}
	}
	return nil
}

func validCommentBody(body string) bool {
	return utf8.ValidString(body) && strings.TrimSpace(body) != "" && utf8.RuneCountInString(body) <= MaxCommentLength
}

func (s *FileCommentService) Create(ctx context.Context, req AccessRequest, input CommentInput, key string) (*models.FileComment, error) {
	if !validCommentBody(input.Body) || (input.Visibility != CommentShared && input.Visibility != CommentTeam) || strings.TrimSpace(key) == "" || len(key) > 128 {
		return nil, ErrCommentInput
	}
	payload, _ := json.Marshal([]string{req.FileID, input.Body, input.Visibility})
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	var comment models.FileComment
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockTenantAccess(tx, req.TenantID); err != nil {
			return err
		}
		actor, team, err := (&FileCommentService{DB: tx}).authorize(ctx, req)
		if err != nil {
			return err
		}
		if input.Visibility == CommentTeam && !team {
			return gorm.ErrRecordNotFound
		}
		err = tx.Where("tenant_id = ? AND author_id = ? AND creation_key = ?", req.TenantID, actor.ID, key).First(&comment).Error
		if err == nil {
			if comment.CreationHash != hash {
				return ErrCommentConflict
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			if err := (&FileCommentService{DB: tx}).validateMentions(ctx, req, actor, input.Visibility, input.Body); err != nil {
				return err
			}
			now := time.Now().UTC()
			comment = models.FileComment{TenantID: req.TenantID, FileID: req.FileID, AuthorID: actor.ID,
				Body: input.Body, Visibility: input.Visibility, Version: 1, CreationKey: key, CreationHash: hash, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&comment).Error; err != nil {
				return err
			}
			if err := recordCommentRevision(tx, comment, "created"); err != nil {
				return err
			}
			if err := recordCommentParticipation(ctx, tx, comment, s.NotificationBaseURL); err != nil {
				return err
			}
		}
		rows := []models.FileComment{comment}
		if err := decorateComments(tx, req, actor, rows); err != nil {
			return err
		}
		comment = rows[0]
		return nil
	})
	return &comment, err
}

// Update edits text only; audience changes require an intentional new comment.
// Optimistic versions prevent one tab from overwriting another's edit/removal.
func (s *FileCommentService) Update(ctx context.Context, req AccessRequest, id, body string, version int, remove bool) (*models.FileComment, error) {
	if version < 1 || (!remove && !validCommentBody(body)) {
		return nil, ErrCommentInput
	}
	var comment models.FileComment
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockTenantAccess(tx, req.TenantID); err != nil {
			return err
		}
		actor, team, err := (&FileCommentService{DB: tx}).authorize(ctx, req)
		if err != nil {
			return err
		}
		if err := commentQuery(tx, req, team).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND author_id = ?", id, actor.ID).First(&comment).Error; err != nil {
			return err
		}
		if comment.Version != version || comment.DeletedAt != nil {
			return ErrCommentConflict
		}
		if !remove {
			if err := (&FileCommentService{DB: tx}).validateMentions(ctx, req, actor, comment.Visibility, body); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		action := "edited"
		if remove {
			action, body = "removed", ""
			comment.DeletedAt = &now
		}
		comment.Body, comment.UpdatedAt, comment.Version = body, now, version+1
		result := tx.Model(&models.FileComment{}).Where("tenant_id = ? AND id = ? AND version = ?", req.TenantID, id, version).
			Updates(map[string]any{"body": body, "updated_at": now, "version": comment.Version, "deleted_at": comment.DeletedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCommentConflict
		}
		if err := recordCommentRevision(tx, comment, action); err != nil {
			return err
		}
		if err := pruneCommentParticipation(tx, comment); err != nil {
			return err
		}
		rows := []models.FileComment{comment}
		if err := decorateComments(tx, req, actor, rows); err != nil {
			return err
		}
		comment = rows[0]
		return nil
	})
	return &comment, err
}

func recordCommentRevision(tx *gorm.DB, comment models.FileComment, action string) error {
	revision := models.FileCommentRevision{TenantID: comment.TenantID, CommentID: comment.ID, Version: comment.Version,
		Body: comment.Body, Action: action, CreatedAt: comment.UpdatedAt}
	if err := tx.Create(&revision).Error; err != nil {
		return err
	}
	// Do not copy conversation text into general-purpose audit/feed payloads.
	payload, err := json.Marshal(map[string]any{"file_id": comment.FileID, "visibility": comment.Visibility, "version": comment.Version})
	if err != nil {
		return err
	}
	return tx.Create(&models.AuditEvent{TenantID: &comment.TenantID, Actor: comment.AuthorID, Action: "file.comment." + action,
		TargetID: &comment.ID, Payload: string(payload), CreatedAt: comment.UpdatedAt}).Error
}

func (s *FileCommentService) History(ctx context.Context, req AccessRequest, id string, before int) ([]models.FileCommentRevision, error) {
	actor, team, err := s.authorize(ctx, req)
	if err != nil {
		return nil, err
	}
	db := s.DB.WithContext(ctx)
	var comment models.FileComment
	if err := commentQuery(db, req, team).Where("id = ? AND author_id = ?", id, actor.ID).First(&comment).Error; err != nil {
		return nil, err
	}
	q := db.Where("tenant_id = ? AND comment_id = ?", req.TenantID, id)
	if before > 0 {
		q = q.Where("version < ?", before)
	}
	revisions := []models.FileCommentRevision{}
	err = q.Order("version DESC").Limit(commentPageSize).Find(&revisions).Error
	if err != nil {
		return nil, err
	}
	for i := range revisions {
		rendered, _, err := RenderCommentMarkdown(revisions[i].Body)
		if err != nil {
			return nil, err
		}
		revisions[i].BodyHTML = &rendered
	}
	return revisions, err
}
