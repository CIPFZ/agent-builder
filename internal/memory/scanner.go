package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Scan(ctx context.Context, projectID, root string) (ScanResult, error) {
	if err := EnsureLayout(root); err != nil {
		return ScanResult{}, err
	}
	var result ScanResult
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			result.Issues = append(result.Issues, ScanIssue{Path: path, Error: err.Error()})
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			result.Issues = append(result.Issues, ScanIssue{Path: path, Error: relErr.Error()})
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.EqualFold(rel, IndexFileName) {
			return nil
		}
		if strings.ToLower(filepath.Ext(rel)) != ".md" {
			return nil
		}
		if _, _, err := ResolveTopicPath(root, rel); err != nil {
			result.Issues = append(result.Issues, ScanIssue{RelativePath: rel, Path: path, Error: err.Error()})
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			result.Issues = append(result.Issues, ScanIssue{RelativePath: rel, Path: path, Error: err.Error()})
			return nil
		}
		if info.Size() > MaxFileBytes {
			result.Issues = append(result.Issues, ScanIssue{RelativePath: rel, Path: path, Error: fmt.Sprintf("memory file exceeds %d bytes", MaxFileBytes)})
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			result.Issues = append(result.Issues, ScanIssue{RelativePath: rel, Path: path, Error: err.Error()})
			return nil
		}
		doc, err := ParseMarkdown(data)
		if err != nil {
			result.Issues = append(result.Issues, ScanIssue{RelativePath: rel, Path: path, Error: err.Error()})
			return nil
		}
		sum := sha256.Sum256(data)
		record := Record{
			ID:                   doc.Frontmatter.ID,
			ProjectID:            projectID,
			RelativePath:         rel,
			AbsolutePath:         path,
			Type:                 doc.Frontmatter.Type,
			Title:                doc.Frontmatter.Title,
			Description:          doc.Frontmatter.Description,
			Tags:                 append([]string(nil), doc.Frontmatter.Tags...),
			ContentHash:          "sha256:" + hex.EncodeToString(sum[:]),
			MTimeUnix:            info.ModTime().Unix(),
			SizeBytes:            info.Size(),
			TokenEstimate:        EstimateTokens(doc.Body),
			Enabled:              true,
			CreatedAt:            doc.Frontmatter.CreatedAt,
			UpdatedAt:            doc.Frontmatter.UpdatedAt,
			CreatedFromSessionID: doc.Frontmatter.SourceSessionID,
			CreatedFromTurnID:    doc.Frontmatter.SourceTurnID,
			LastIndexedAt:        NowRFC3339(),
			Preview:              Preview(doc.Body, 320),
		}
		if record.CreatedAt == "" {
			record.CreatedAt = record.LastIndexedAt
		}
		if record.UpdatedAt == "" {
			record.UpdatedAt = record.LastIndexedAt
		}
		result.Records = append(result.Records, ScannedRecord{Record: record, Content: doc.Body})
		return nil
	})
	return result, err
}
