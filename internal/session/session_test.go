package session

import (
	"testing"

	"github.com/CIPFZ/agent-builder/internal/db"
	"github.com/stretchr/testify/require"
)

func TestEstimatedUsageStateSurvivesFetchModifySave(t *testing.T) {
	dataDir := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, db.Release(dataDir))
		db.ResetPool()
	})

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)

	sessions := NewService(db.New(conn), conn)

	created, err := sessions.Create(t.Context(), "test")
	require.NoError(t, err)
	created.PromptTokens = 100
	created.CompletionTokens = 50
	created.EstimatedUsage = true

	saved, err := sessions.Save(t.Context(), created)
	require.NoError(t, err)
	require.True(t, saved.EstimatedUsage)

	fetched, err := sessions.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.True(t, fetched.EstimatedUsage)

	fetched.Todos = []Todo{{
		Content:    "Check estimate state",
		Status:     TodoStatusInProgress,
		ActiveForm: "Checking estimate state",
	}}

	updated, err := sessions.Save(t.Context(), fetched)
	require.NoError(t, err)
	require.True(t, updated.EstimatedUsage)

	refetched, err := sessions.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.True(t, refetched.EstimatedUsage)
}

func TestEstimatedUsageStateCanBeClearedByExplicitSave(t *testing.T) {
	dataDir := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, db.Release(dataDir))
		db.ResetPool()
	})

	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)

	sessions := NewService(db.New(conn), conn)

	created, err := sessions.Create(t.Context(), "test")
	require.NoError(t, err)
	created.PromptTokens = 100
	created.CompletionTokens = 50
	created.EstimatedUsage = true

	saved, err := sessions.Save(t.Context(), created)
	require.NoError(t, err)
	require.True(t, saved.EstimatedUsage)

	saved.EstimatedUsage = false
	updated, err := sessions.Save(t.Context(), saved)
	require.NoError(t, err)
	require.False(t, updated.EstimatedUsage)

	refetched, err := sessions.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.False(t, refetched.EstimatedUsage)
}

func TestFinalizeGeneratedTitleDoesNotOverrideUserRename(t *testing.T) {
	dataDir := t.TempDir()
	t.Cleanup(func() {
		require.NoError(t, db.Release(dataDir))
		db.ResetPool()
	})
	conn, err := db.Connect(t.Context(), dataDir)
	require.NoError(t, err)
	sessions := NewService(db.New(conn), conn)

	created, err := sessions.Create(t.Context(), "fallback title")
	require.NoError(t, err)
	created.TitleSource = TitleSourceFallbackPending
	created, err = sessions.Save(t.Context(), created)
	require.NoError(t, err)

	applied, err := sessions.FinalizeGeneratedTitle(t.Context(), created.ID, created.Title, "generated title", TitleSourceAgent, 2, 3, 0.5)
	require.NoError(t, err)
	require.True(t, applied)
	generated, err := sessions.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Equal(t, TitleSourceAgent, generated.TitleSource)
	require.Equal(t, int64(2), generated.PromptTokens)
	require.Equal(t, int64(3), generated.CompletionTokens)

	generated.Title = "user title"
	generated.TitleSource = TitleSourceUser
	_, err = sessions.Save(t.Context(), generated)
	require.NoError(t, err)
	applied, err = sessions.FinalizeGeneratedTitle(t.Context(), created.ID, "generated title", "late title", TitleSourceAgent, 10, 10, 1)
	require.NoError(t, err)
	require.False(t, applied)
	final, err := sessions.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "user title", final.Title)
	require.Equal(t, TitleSourceUser, final.TitleSource)
	require.Equal(t, int64(2), final.PromptTokens)
	require.Equal(t, int64(3), final.CompletionTokens)
}
