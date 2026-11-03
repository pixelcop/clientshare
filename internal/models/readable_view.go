package models

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReadableView survives client deletion and owns the generated manifest.
type ReadableView struct {
	TenantID        string `gorm:"primaryKey"`
	ClientID        string `gorm:"primaryKey"`
	DesiredRevision int64  `gorm:"not null;default:1"`
	AppliedRevision int64  `gorm:"not null;default:0"`
	LastError       string
	Manifest        string
}

func DirtyView(tx *gorm.DB, tenantID, clientID string) error {
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "client_id"}}, DoUpdates: clause.Assignments(map[string]interface{}{"desired_revision": gorm.Expr("readable_views.desired_revision + 1")})}).Create(&ReadableView{TenantID: tenantID, ClientID: clientID, DesiredRevision: 1}).Error
}
