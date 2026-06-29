package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	store     Store
	projectID string
	root      string
}

func NewService(store Store, projectID, root string) Service {
	return Service{store: store, projectID: strings.TrimSpace(projectID), root: root}
}

type WriteRequest struct {
	RelativePath    string
	Type            string
	Title           string
	Description     string
	Tags            []string
	Content         string
	SourceSessionID string
	SourceTurnID    string
	Confidence      float64
	PreserveID      string
}

func (s Service) RebuildIndex(ctx context.Context) (IndexResult, error) {
	started := NowRFC3339()
	scan, err := Scan(ctx, s.projectID, s.root)
	if err != nil {
		return IndexResult{}, err
	}
	seen := map[string]struct{}{}
	indexed := 0
	for _, scanned := range scan.Records {
		seen[scanned.Record.ID] = struct{}{}
		seen[scanned.Record.RelativePath] = struct{}{}
		if _, err := s.store.UpsertRecord(ctx, scanned.Record); err != nil {
			scan.Issues = append(scan.Issues, ScanIssue{RelativePath: scanned.Record.RelativePath, Path: scanned.Record.AbsolutePath, Error: err.Error()})
			continue
		}
		indexed++
	}
	deleted, err := s.store.MarkMissingDeleted(ctx, s.projectID, seen, NowRFC3339())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return IndexResult{}, err
	}
	_ = s.rewriteIndex(ctx)
	return IndexResult{ProjectID: s.projectID, Indexed: indexed, Deleted: deleted, Failed: len(scan.Issues), Issues: scan.Issues, StartedAt: started, EndedAt: NowRFC3339()}, nil
}

func (s Service) List(ctx context.Context, includeDeleted bool) ([]Record, error) {
	if err := s.RebuildIfEmpty(ctx); err != nil {
		return nil, err
	}
	records, err := s.store.ListByProject(ctx, s.projectID, includeDeleted)
	if err != nil {
		return nil, err
	}
	for i := range records {
		records[i].AbsolutePath = filepath.Join(s.root, filepath.FromSlash(records[i].RelativePath))
		if content, err := s.ReadContent(records[i]); err == nil {
			records[i].Preview = Preview(content, 320)
		}
	}
	return records, nil
}

func (s Service) Get(ctx context.Context, id string) (Record, error) {
	record, err := s.store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	record.AbsolutePath = filepath.Join(s.root, filepath.FromSlash(record.RelativePath))
	content, err := s.ReadContent(record)
	if err != nil {
		return Record{}, err
	}
	record.Content = content
	record.Preview = Preview(content, 320)
	return record, nil
}

func (s Service) Create(ctx context.Context, req WriteRequest) (Record, error) {
	now := NowRFC3339()
	id := firstNonEmpty(strings.TrimSpace(req.PreserveID), newMemoryID())
	if strings.TrimSpace(req.RelativePath) == "" {
		req.RelativePath = filepath.ToSlash(filepath.Join(strings.ToLower(strings.TrimSpace(req.Type)), Slug(req.Title)+".md"))
	}
	return s.write(ctx, id, req, now, now)
}

func (s Service) Update(ctx context.Context, id string, req WriteRequest) (Record, error) {
	existing, err := s.store.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if strings.TrimSpace(req.RelativePath) == "" {
		req.RelativePath = existing.RelativePath
	}
	if strings.TrimSpace(req.Type) == "" {
		req.Type = existing.Type
	}
	if strings.TrimSpace(req.Title) == "" {
		req.Title = existing.Title
	}
	if strings.TrimSpace(req.Description) == "" {
		req.Description = existing.Description
	}
	return s.write(ctx, existing.ID, req, existing.CreatedAt, NowRFC3339())
}

func (s Service) Disable(ctx context.Context, id string, enabled bool) (Record, error) {
	record, err := s.store.SetEnabled(ctx, id, enabled, NowRFC3339())
	if err == nil {
		_ = s.rewriteIndex(ctx)
	}
	return record, err
}

func (s Service) Delete(ctx context.Context, id string) (Record, error) {
	record, err := s.store.Delete(ctx, id, NowRFC3339())
	if err == nil {
		_ = s.rewriteIndex(ctx)
	}
	return record, err
}

func (s Service) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	req.ProjectID = s.projectID
	records, err := s.List(ctx, false)
	if err != nil {
		return nil, err
	}
	bodies := map[string]string{}
	for _, record := range records {
		body, err := s.ReadContent(record)
		if err == nil {
			bodies[record.ID] = body
		}
	}
	return Rank(records, bodies, req), nil
}

func (s Service) RecordInjection(ctx context.Context, injection Injection) error {
	if strings.TrimSpace(injection.ID) == "" {
		injection.ID = "minj_" + strings.TrimPrefix(newMemoryID(), "mem_")
	}
	if injection.InjectedAt == "" {
		injection.InjectedAt = NowRFC3339()
	}
	return s.store.InsertInjection(ctx, injection)
}

func (s Service) ReadContent(record Record) (string, error) {
	path, _, err := ResolveTopicPath(s.root, record.RelativePath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > MaxFileBytes {
		return "", fmt.Errorf("memory file exceeds %d bytes", MaxFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	doc, err := ParseMarkdown(data)
	if err != nil {
		return "", err
	}
	return doc.Body, nil
}

func (s Service) RebuildIfEmpty(ctx context.Context) error {
	if _, err := os.Stat(s.root); os.IsNotExist(err) {
		_, err = s.RebuildIndex(ctx)
		return err
	}
	return nil
}

func (s Service) write(ctx context.Context, id string, req WriteRequest, createdAt, updatedAt string) (Record, error) {
	if err := EnsureLayout(s.root); err != nil {
		return Record{}, err
	}
	target, rel, err := ResolveTopicPath(s.root, req.RelativePath)
	if err != nil {
		return Record{}, err
	}
	fm := NormalizeFrontmatter(Frontmatter{
		ID:              id,
		Title:           req.Title,
		Type:            req.Type,
		Description:     req.Description,
		Tags:            req.Tags,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		SourceSessionID: strings.TrimSpace(req.SourceSessionID),
		SourceTurnID:    strings.TrimSpace(req.SourceTurnID),
		Confidence:      req.Confidence,
	})
	data, err := RenderMarkdown(Document{Frontmatter: fm, Body: req.Content})
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return Record{}, fmt.Errorf("failed to create memory parent directory: %w", err)
	}
	tmp := target + fmt.Sprintf(".%d.tmp", time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return Record{}, fmt.Errorf("failed to write memory file: %w", err)
	}
	if err := replaceFile(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return Record{}, fmt.Errorf("failed to replace memory file: %w", err)
	}
	scan, err := Scan(ctx, s.projectID, s.root)
	if err != nil {
		return Record{}, err
	}
	for _, scanned := range scan.Records {
		if scanned.Record.ID != id {
			continue
		}
		scanned.Record.RelativePath = rel
		record, err := s.store.UpsertRecord(ctx, scanned.Record)
		if err == nil {
			_ = s.rewriteIndex(ctx)
		}
		return record, err
	}
	return Record{}, errors.New("written memory file was not indexed")
}

func replaceFile(tmp, target string) error {
	if err := os.Rename(tmp, target); err == nil {
		return nil
	}
	if _, statErr := os.Stat(target); statErr == nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return removeErr
		}
		return os.Rename(tmp, target)
	}
	return os.Rename(tmp, target)
}

func (s Service) rewriteIndex(ctx context.Context) error {
	records, err := s.store.ListByProject(ctx, s.projectID, false)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Project Memory\n\n")
	count := 0
	for _, record := range records {
		if !record.Enabled || record.DeletedAt != "" {
			continue
		}
		fmt.Fprintf(&b, "- [%s](%s) - %s\n", record.Title, filepath.ToSlash(record.RelativePath), record.Description)
		count++
		if count >= 200 || b.Len() > 25*1024 {
			break
		}
	}
	return os.WriteFile(filepath.Join(s.root, IndexFileName), []byte(b.String()), 0o600)
}

func newMemoryID() string {
	var data [10]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("mem_%d", time.Now().UnixNano())
	}
	return "mem_" + hex.EncodeToString(data[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
