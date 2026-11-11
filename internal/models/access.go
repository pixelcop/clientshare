package models

// AccessCapabilities are independent rights. Upload never implies delete/share.
// The embedded columns are shared by memberships and named item grants.
type AccessCapabilities struct {
	CanRead   bool `gorm:"not null;default:false" json:"can_read"`
	CanUpload bool `gorm:"not null;default:false" json:"can_upload"`
	CanDelete bool `gorm:"not null;default:false" json:"can_delete"`
	CanShare  bool `gorm:"not null;default:false" json:"can_share"`
	// Nil is the pre-role compatibility contract. Role-backed capabilities carry
	// independent sharing rights; never infer them from the summary CanShare bit.
	Sharing *SharingPermissions `gorm:"-" json:"sharing,omitempty"`
}

type SharingPermissions struct {
	InviteExternal bool `json:"invite_external"`
	CreateLinks    bool `json:"create_links"`
	ManageAccess   bool `json:"manage_access"`
}

func (c AccessCapabilities) Permissions() RolePermissions {
	p := RolePermissions{CanRead: c.CanRead, CanUpload: c.CanUpload, CanDelete: c.CanDelete,
		CanInviteExternal: c.CanShare, CanCreateLinks: c.CanShare, CanManageAccess: c.CanShare}
	if c.Sharing != nil {
		p.CanInviteExternal, p.CanCreateLinks, p.CanManageAccess = c.Sharing.InviteExternal, c.Sharing.CreateLinks, c.Sharing.ManageAccess
	}
	return p
}

func (p RolePermissions) Capabilities() AccessCapabilities {
	return AccessCapabilities{CanRead: p.CanRead, CanUpload: p.CanUpload, CanDelete: p.CanDelete,
		CanShare: p.CanInviteExternal || p.CanCreateLinks || p.CanManageAccess,
		Sharing:  &SharingPermissions{InviteExternal: p.CanInviteExternal, CreateLinks: p.CanCreateLinks, ManageAccess: p.CanManageAccess}}
}

func SameAccessCapabilities(a, b AccessCapabilities) bool { return a.Permissions() == b.Permissions() }
