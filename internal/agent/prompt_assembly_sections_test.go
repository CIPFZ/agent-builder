package agent

import (
	"strings"
	"testing"
)

func TestAppendRuntimePromptSectionsProviderPrefixAndMCPDelta(t *testing.T) {
	sections := appendRuntimePromptSections(nil, "provider prefix", []mcpInstructionSnapshot{{
		Server:  "docs",
		Content: "Use docs search before generic search.",
	}})
	if len(sections) != 2 {
		t.Fatalf("sections = %#v", sections)
	}
	if sections[0].Kind != "provider_prefix" || sections[0].CachePolicy != "session_cached" {
		t.Fatalf("provider prefix section = %#v", sections[0])
	}
	mcpSection := sections[1]
	if mcpSection.Kind != "mcp_instructions_delta" || mcpSection.CachePolicy != "uncached" {
		t.Fatalf("mcp section = %#v", mcpSection)
	}
	if mcpSection.Hash != hashPromptText("Use docs search before generic search.") {
		t.Fatalf("mcp hash must use instruction content: %#v", mcpSection)
	}
	if mcpSection.Hash == hashPromptText("docs") {
		t.Fatalf("mcp hash incorrectly used server name: %#v", mcpSection)
	}
	names := mcpInstructionServerNames([]mcpInstructionSnapshot{{Server: "zeta", Content: "Z"}, {Server: "alpha", Content: "A"}})
	if got, want := hashStringList(names), hashPromptText("alpha\nzeta"); got != want {
		t.Fatalf("server list hash = %s, want %s", got, want)
	}
	contents := strings.Join(mcpInstructionContents([]mcpInstructionSnapshot{{Server: "zeta", Content: "Z"}, {Server: "alpha", Content: "A"}}), "\n\n")
	if got, want := hashPromptText(contents), hashPromptText("A\n\nZ"); got != want {
		t.Fatalf("instruction content hash = %s, want %s", got, want)
	}
	if !mcpSection.Redacted || mcpSection.RawStored {
		t.Fatalf("mcp raw content flags = %#v", mcpSection)
	}

	ordered := appendRuntimePromptSections([]PromptSectionSummary{{
		ID:          "stable_base",
		Kind:        "stable_base",
		Role:        "system",
		Order:       1,
		CachePolicy: "stable",
	}}, "provider prefix", nil)
	if len(ordered) != 2 || ordered[0].Kind != "provider_prefix" || ordered[0].Order != 1 || ordered[1].Kind != "stable_base" || ordered[1].Order != 2 {
		t.Fatalf("runtime section order = %#v", ordered)
	}
}

func TestPromptSkillSummaryExtractsAvailableSkillsXML(t *testing.T) {
	summary := promptSkillSummaryFromSystem(`<available_skills>
  <skill>
    <name>agent-builder-config</name>
    <description>Configure Agent Builder.</description>
  </skill>
</available_skills>`)
	if summary.AvailableCount != 1 || summary.LoadedNames[0] != "agent-builder-config" || summary.XMLHash == "" {
		t.Fatalf("skill summary = %#v", summary)
	}
}
