package access

import (
	"context"
	"path"

	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

// ItemNavigation returns a contiguous authorized chain (root first), never a
// private ancestor used merely to validate inheritance. Readable targets cannot
// use upload-only ancestors to browse existing content.
func (s *AccessService) ItemNavigation(ctx context.Context, req AccessRequest) ([]models.File, error) {
	nodes, err := s.loadAccessAncestry(ctx, req, []string{req.FileID})
	if err != nil {
		return nil, err
	}
	chain, valid := accessChain(nodes, req.FileID)
	if !valid {
		return nil, gorm.ErrRecordNotFound
	}
	ids := make([]string, len(chain))
	for i := range chain {
		ids[i] = chain[i].ID
	}
	req.Operation = AccessContext
	decisions, err := s.ResolveItems(ctx, req, ids)
	if err != nil {
		return nil, err
	}
	if !decisions[req.FileID].Allowed {
		return nil, gorm.ErrRecordNotFound
	}
	read := decisions[req.FileID].Capabilities.CanRead
	var visible []models.File
	for _, file := range chain {
		d := decisions[file.ID]
		if !d.Allowed || (read && !d.Capabilities.CanRead) || (!read && !d.Capabilities.CanUpload) {
			break
		}
		file.Capabilities = &d.Capabilities
		visible = append(visible, file)
	}
	for i, j := 0, len(visible)-1; i < j; i, j = i+1, j-1 {
		visible[i], visible[j] = visible[j], visible[i]
	}
	root := visible[0].ID
	if !read {
		// An upload-only context is not metadata permission. Keep only the
		// selected destination; never serialize ancestor names or paths.
		file := visible[len(visible)-1]
		file.Filename, file.Path, file.FolderID, file.UserID = "Upload destination", "", nil, nil
		file.NavigationRootID = root
		return []models.File{file}, nil
	}
	logicalPath := ""
	for i := range visible {
		logicalPath = path.Join(logicalPath, visible[i].Filename)
		visible[i].Path = logicalPath
		visible[i].NavigationRootID = root
		visible[i].UserID = nil
		if i == 0 {
			visible[i].FolderID = nil
		}
	}
	return visible, nil
}

// DescribeItem is a recipient-safe DTO; raw File.Path and private parent IDs
// must not be serialized for an exact-item grantee.
func (s *AccessService) DescribeItem(ctx context.Context, req AccessRequest) (*models.File, error) {
	chain, err := s.ItemNavigation(ctx, req)
	if err != nil {
		return nil, err
	}
	return &chain[len(chain)-1], nil
}
