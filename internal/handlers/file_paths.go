package handlers

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/pixelcop/clientshare/internal/models"
)

func resolveClientPath(root, requested string) (string, error) {
	rootClean := filepath.Clean(root)
	trimmed := strings.TrimSpace(requested)
	if trimmed == "" || trimmed == "." || trimmed == string(filepath.Separator) {
		return rootClean, nil
	}
	trimmed = strings.TrimPrefix(trimmed, string(filepath.Separator))
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return rootClean, nil
	}
	abs := filepath.Join(rootClean, cleaned)
	if abs == rootClean {
		return rootClean, nil
	}
	if !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) {
		return "", errors.New("invalid path")
	}
	return abs, nil
}

func filterDirectChildren(items []models.File, parentPath string) []models.File {
	parentClean := filepath.Clean(parentPath)
	filtered := make([]models.File, 0, len(items))
	for _, item := range items {
		itemPath := filepath.Clean(item.Path)
		if filepath.Dir(itemPath) == parentClean {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func paginateFiles(items []models.File, page, pageSize int) []models.File {
	if pageSize <= 0 {
		return items
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []models.File{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
