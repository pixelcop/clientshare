package models

// AccessProfile separates organizational identity from scoped document access.
// The legacy role remains authoritative until the permission resolver cutover.
// CanManageUsers preserves existing managers' administrative powers explicitly;
// new Members do not receive it by default.
type AccessProfile struct {
	TenantID         string `gorm:"primaryKey" json:"tenant_id"`
	UserID           string `gorm:"primaryKey;size:26" json:"user_id"`
	UserType         string `gorm:"not null" json:"user_type"`
	OrganizationRole string `gorm:"not null" json:"organization_role"`
	CanManageUsers   bool   `gorm:"not null;default:false" json:"can_manage_users"`
	AllWorkspaces    bool   `gorm:"not null;default:false" json:"all_workspaces"`
}
