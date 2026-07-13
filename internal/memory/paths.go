package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func EnsureLayout(root string) error {
	if root == "" {
		return errors.New("memory root is required")
	}
	for _, dir := range []string{"", TypeUser, TypeFeedback, TypeProject, TypeReference} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			return fmt.Errorf("failed to create memory directory: %w", err)
		}
	}
	index := filepath.Join(root, IndexFileName)
	if _, err := os.Stat(index); os.IsNotExist(err) {
		if err := os.WriteFile(index, []byte("# Project Memory\n\n"), 0o600); err != nil {
			return fmt.Errorf("failed to create memory index: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to read memory index: %w", err)
	}
	return nil
}

func ResolveTopicPath(root, relativePath string) (string, string, error) {
	root = filepath.Clean(root)
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return "", "", errors.New("memory relative path is required")
	}
	if strings.ContainsRune(relativePath, 0) {
		return "", "", errors.New("memory relative path cannot contain NUL")
	}
	if filepath.IsAbs(relativePath) || looksLikeWindowsAbs(relativePath) {
		return "", "", errors.New("memory relative path must not be absolute")
	}
	slash := filepath.ToSlash(relativePath)
	parts := strings.Split(slash, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", "", errors.New("memory relative path contains an unsafe segment")
		}
	}
	if strings.EqualFold(filepath.Base(slash), IndexFileName) {
		return "", "", errors.New("MEMORY.md is an index, not a topic memory")
	}
	if strings.ToLower(filepath.Ext(slash)) != ".md" {
		return "", "", errors.New("memory topic files must use .md extension")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(slash))
	target := filepath.Join(root, cleanRel)
	if !isInside(root, target) {
		return "", "", errors.New("memory path escapes memory root")
	}
	if err := rejectSymlinkEscape(root, target); err != nil {
		return "", "", err
	}
	return target, filepath.ToSlash(cleanRel), nil
}

func rejectSymlinkEscape(root, target string) error {
	evalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		if os.IsNotExist(err) {
			evalRoot = root
		} else {
			return fmt.Errorf("failed to evaluate memory root: %w", err)
		}
	}
	check := target
	if _, err := os.Lstat(check); os.IsNotExist(err) {
		check = filepath.Dir(check)
	}
	for {
		if _, err := os.Lstat(check); err == nil {
			break
		}
		next := filepath.Dir(check)
		if next == check {
			break
		}
		check = next
	}
	evalTarget, err := filepath.EvalSymlinks(check)
	if err != nil {
		return fmt.Errorf("failed to evaluate memory path: %w", err)
	}
	if !isInside(evalRoot, evalTarget) {
		return errors.New("memory path escapes memory root through a symlink")
	}
	return nil
}

func isInside(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if root == target {
		return true
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func looksLikeWindowsAbs(path string) bool {
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

func Slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out strings.Builder
	lastDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(out.String(), "-")
	if slug == "" {
		return "memory"
	}
	return slug
}
