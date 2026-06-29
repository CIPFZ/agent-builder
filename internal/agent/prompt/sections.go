package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CIPFZ/agent-builder/internal/skills"
)

const (
	CachePolicyStable        = "stable"
	CachePolicySessionCached = "session_cached"
	CachePolicyTurnDynamic   = "turn_dynamic"
	CachePolicyUncached      = "uncached"
)

type PromptSection struct {
	ID            string
	Name          string
	Kind          string
	Role          string
	Order         int
	CachePolicy   string
	Source        string
	SourceRefs    []string
	Scope         string
	Content       string
	Hash          string
	Length        int
	TokenEstimate int
	Redacted      bool
	RawStored     bool
	Diagnostics   string
}

type sectionSpec struct {
	id          string
	name        string
	kind        string
	role        string
	order       int
	cachePolicy string
	source      string
	sourceRefs  []string
	scope       string
	content     string
	diagnostics string
}

func newSection(spec sectionSpec) PromptSection {
	policy := strings.TrimSpace(spec.cachePolicy)
	if policy == "" {
		policy = CachePolicyTurnDynamic
	}
	role := strings.TrimSpace(spec.role)
	if role == "" {
		role = "system"
	}
	section := PromptSection{
		ID:            strings.TrimSpace(spec.id),
		Name:          strings.TrimSpace(spec.name),
		Kind:          strings.TrimSpace(spec.kind),
		Role:          role,
		Order:         spec.order,
		CachePolicy:   policy,
		Source:        strings.TrimSpace(spec.source),
		SourceRefs:    compactStrings(spec.sourceRefs),
		Scope:         strings.TrimSpace(spec.scope),
		Content:       spec.content,
		Hash:          hashSectionContent(spec.content),
		Length:        len(spec.content),
		TokenEstimate: skills.ApproxTokenCount(spec.content),
		Redacted:      true,
		RawStored:     false,
		Diagnostics:   strings.TrimSpace(spec.diagnostics),
	}
	if section.ID == "" {
		section.ID = sectionID(section.Kind, section.Name, section.Order)
	}
	if section.Name == "" {
		section.Name = section.Kind
	}
	if section.Kind == "" {
		section.Kind = "prompt_section"
	}
	if section.Scope == "" {
		section.Scope = "runtime"
	}
	return section
}

func hashSectionContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

var sectionIDPattern = regexp.MustCompile(`[^a-zA-Z0-9:_./-]+`)

func sectionID(kind, name string, order int) string {
	base := strings.ToLower(strings.TrimSpace(kind + ":" + name))
	base = filepath.ToSlash(base)
	base = sectionIDPattern.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "prompt_section"
	}
	return fmt.Sprintf("%03d:%s", order, base)
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
