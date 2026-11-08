package models

// AccessCapabilities are independent rights. Upload never implies delete/share.
// The embedded columns are shared by memberships and named item grants.
type AccessCapabilities struct {
	CanRead   bool `gorm:"not null;default:false" json:"can_read"`
	CanUpload bool `gorm:"not null;default:false" json:"can_upload"`
	CanDelete bool `gorm:"not null;default:false" json:"can_delete"`
	CanShare  bool `gorm:"not null;default:false" json:"can_share"`
}
