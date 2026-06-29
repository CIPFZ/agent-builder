package memory

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

func ParseMarkdown(data []byte) (Document, error) {
	if !utf8.Valid(data) {
		return Document{}, errors.New("memory markdown is not valid UTF-8")
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return Document{}, errors.New("memory markdown frontmatter is required")
	}
	bodyStart := -1
	for _, marker := range []string{"\n---\n", "\r\n---\r\n", "\n---\r\n", "\r\n---\n"} {
		if idx := strings.Index(text[len("---"):], marker); idx >= 0 {
			bodyStart = len("---") + idx + len(marker)
			break
		}
	}
	if bodyStart < 0 {
		return Document{}, errors.New("memory markdown frontmatter is not closed")
	}
	header := text[len("---"):bodyStart]
	if idx := strings.LastIndex(header, "---"); idx >= 0 {
		header = header[:idx]
	}
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(header)), &fm); err != nil {
		return Document{}, fmt.Errorf("failed to parse memory frontmatter: %w", err)
	}
	fm = NormalizeFrontmatter(fm)
	if err := ValidateFrontmatter(fm); err != nil {
		return Document{}, err
	}
	return Document{Frontmatter: fm, Body: strings.TrimLeft(text[bodyStart:], "\r\n")}, nil
}

func RenderMarkdown(doc Document) ([]byte, error) {
	fm := NormalizeFrontmatter(doc.Frontmatter)
	if err := ValidateFrontmatter(fm); err != nil {
		return nil, err
	}
	header, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("failed to render memory frontmatter: %w", err)
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(header)
	out.WriteString("---\n\n")
	out.WriteString(strings.TrimLeft(doc.Body, "\r\n"))
	if !strings.HasSuffix(out.String(), "\n") {
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func NormalizeFrontmatter(fm Frontmatter) Frontmatter {
	fm.ID = strings.TrimSpace(fm.ID)
	fm.Title = strings.TrimSpace(fm.Title)
	fm.Type = strings.ToLower(strings.TrimSpace(fm.Type))
	fm.Description = strings.TrimSpace(fm.Description)
	fm.CreatedAt = strings.TrimSpace(fm.CreatedAt)
	fm.UpdatedAt = strings.TrimSpace(fm.UpdatedAt)
	fm.SourceSessionID = strings.TrimSpace(fm.SourceSessionID)
	fm.SourceTurnID = strings.TrimSpace(fm.SourceTurnID)
	tags := make([]string, 0, len(fm.Tags))
	seen := map[string]struct{}{}
	for _, tag := range fm.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	fm.Tags = tags
	return fm
}

func ValidateFrontmatter(fm Frontmatter) error {
	if fm.ID == "" {
		return errors.New("memory frontmatter id is required")
	}
	if fm.Title == "" {
		return errors.New("memory frontmatter title is required")
	}
	if fm.Type == "" {
		return errors.New("memory frontmatter type is required")
	}
	if _, ok := ValidTypes[fm.Type]; !ok {
		return fmt.Errorf("unsupported memory type %q", fm.Type)
	}
	if fm.Description == "" {
		return errors.New("memory frontmatter description is required")
	}
	return nil
}

func EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return (utf8.RuneCountInString(text) + 3) / 4
}

func Preview(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}
