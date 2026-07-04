package agent

import (
	"testing"

	"github.com/CIPFZ/agent-builder/internal/hooks"
	"github.com/stretchr/testify/require"
)

func TestExtractAdditionalInstructionsJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "empty", payload: "", want: ""},
		{name: "not_json", payload: "hello world", want: ""},
		{name: "no_field", payload: `{"decision":"allow"}`, want: ""},
		{name: "present", payload: `{"additionalInstructions":"preserve error handling"}`, want: "preserve error handling"},
		{name: "trims_whitespace", payload: `{"additionalInstructions":"  keep the tests  "}`, want: "keep the tests"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractAdditionalInstructionsJSON(tc.payload)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestCompactHookAdditionalInstructionsPrefersUpdatedInput(t *testing.T) {
	t.Parallel()

	agg := hooks.AggregateResult{
		UpdatedInput: `{"additionalInstructions":"from updated_input"}`,
		Context:      "from context block",
	}
	require.Equal(t, "from updated_input", compactHookAdditionalInstructions(agg))
}

func TestCompactHookAdditionalInstructionsFallsBackToContext(t *testing.T) {
	t.Parallel()

	agg := hooks.AggregateResult{
		UpdatedInput: `{"decision":"allow"}`,
		Context:      "keep the http router intact",
	}
	require.Equal(t, "keep the http router intact", compactHookAdditionalInstructions(agg))
}

func TestCompactHookAdditionalInstructionsReturnsEmpty(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", compactHookAdditionalInstructions(hooks.AggregateResult{}))
}
