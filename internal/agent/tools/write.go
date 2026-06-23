package tools

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/CIPFZ/agent-builder/internal/diff"
	"github.com/CIPFZ/agent-builder/internal/filepathext"
	"github.com/CIPFZ/agent-builder/internal/filetracker"
	"github.com/CIPFZ/agent-builder/internal/fsext"
	"github.com/CIPFZ/agent-builder/internal/history"

	"github.com/CIPFZ/agent-builder/internal/lsp"
	"github.com/CIPFZ/agent-builder/internal/permission"
)

//go:embed write.md
var writeDescription string

type WriteParams struct {
	FilePath string `json:"file_path" description:"The path to the file to write"`
	Content  string `json:"content" description:"The content to write to the file"`
	Mode     string `json:"mode,omitempty" description:"Write mode: overwrite (default) or append"`
}

type WritePermissionsParams struct {
	FilePath   string `json:"file_path"`
	Mode       string `json:"mode,omitempty"`
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
}

type WriteResponseMetadata struct {
	Diff      string `json:"diff"`
	Additions int    `json:"additions"`
	Removals  int    `json:"removals"`
}

const WriteToolName = "write"

func NewWriteTool(
	lspManager *lsp.Manager,
	permissions permission.Service,
	files history.Service,
	filetracker filetracker.Service,
	workingDir string,
) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		WriteToolName,
		writeDescription,
		func(ctx context.Context, params WriteParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.FilePath == "" {
				return fantasy.NewTextErrorResponse("file_path is required"), nil
			}
			mode := normalizeWriteMode(params.Mode)
			if mode == "" {
				return fantasy.NewTextErrorResponse("mode must be either overwrite or append"), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session_id is required")
			}

			effectiveDir := effectiveWorkingDir(ctx, workingDir)
			filePath := filepathext.SmartJoin(effectiveDir, params.FilePath)

			fileInfo, err := os.Stat(filePath)
			if err == nil {
				if fileInfo.IsDir() {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("Path is a directory, not a file: %s", filePath)), nil
				}

				modTime := fileInfo.ModTime().Truncate(time.Second)
				lastRead := filetracker.LastReadTime(ctx, sessionID, filePath)
				if modTime.After(lastRead) {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("File %s has been modified since it was last read.\nLast modification: %s\nLast read: %s\n\nPlease read the file again before modifying it.",
						filePath, modTime.Format(time.RFC3339), lastRead.Format(time.RFC3339))), nil
				}

				oldContent, readErr := os.ReadFile(filePath)
				if mode == "overwrite" && readErr == nil && string(oldContent) == params.Content {
					return fantasy.NewTextErrorResponse(fmt.Sprintf("File %s already contains the exact content. No changes made.", filePath)), nil
				}
			} else if !os.IsNotExist(err) {
				return fantasy.ToolResponse{}, fmt.Errorf("error checking file: %w", err)
			}

			dir := filepath.Dir(filePath)
			if err = os.MkdirAll(dir, 0o755); err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("error creating directory: %w", err)
			}

			oldContent := ""
			if fileInfo != nil && !fileInfo.IsDir() {
				oldBytes, readErr := os.ReadFile(filePath)
				if readErr == nil {
					oldContent = string(oldBytes)
				}
			}
			newContent := params.Content
			if mode == "append" {
				newContent = oldContent + params.Content
			}

			diff, additions, removals := diff.GenerateDiff(
				oldContent,
				newContent,
				strings.TrimPrefix(filePath, effectiveDir),
			)

			p, err := permissions.Request(ctx,
				permission.CreatePermissionRequest{
					SessionID:   sessionID,
					Path:        fsext.PathOrPrefix(filePath, effectiveDir),
					ToolCallID:  call.ID,
					ToolName:    WriteToolName,
					Action:      "write",
					Description: fmt.Sprintf("Create file %s", filePath),
					Params: WritePermissionsParams{
						FilePath:   filePath,
						Mode:       mode,
						OldContent: oldContent,
						NewContent: newContent,
					},
				},
			)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !p {
				return NewPermissionDeniedResponse(), nil
			}

			if mode == "append" {
				file, openErr := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
				if openErr != nil {
					return fantasy.ToolResponse{}, fmt.Errorf("error opening file for append: %w", openErr)
				}
				if _, err = file.WriteString(params.Content); err != nil {
					_ = file.Close()
					return fantasy.ToolResponse{}, fmt.Errorf("error appending file: %w", err)
				}
				if err = file.Close(); err != nil {
					return fantasy.ToolResponse{}, fmt.Errorf("error closing file: %w", err)
				}
			} else {
				err = os.WriteFile(filePath, []byte(params.Content), 0o644)
				if err != nil {
					return fantasy.ToolResponse{}, fmt.Errorf("error writing file: %w", err)
				}
			}

			// Check if file exists in history
			file, err := files.GetByPathAndSession(ctx, filePath, sessionID)
			if err != nil {
				_, err = files.Create(ctx, sessionID, filePath, oldContent)
				if err != nil {
					// Log error but don't fail the operation
					return fantasy.ToolResponse{}, fmt.Errorf("error creating file history: %w", err)
				}
			}
			if file.Content != oldContent {
				// User manually changed the content; store an intermediate version
				_, err = files.CreateVersion(ctx, sessionID, filePath, oldContent)
				if err != nil {
					slog.Error("Error creating file history version", "error", err)
				}
			}
			// Store the new version
			_, err = files.CreateVersion(ctx, sessionID, filePath, newContent)
			if err != nil {
				slog.Error("Error creating file history version", "error", err)
			}

			filetracker.RecordRead(ctx, sessionID, filePath)

			notifyLSPs(ctx, lspManager, params.FilePath)

			result := fmt.Sprintf("File successfully %s: %s", writeModePastTense(mode), filePath)
			result = fmt.Sprintf("<result>\n%s\n</result>", result)
			result += getDiagnostics(filePath, lspManager)
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result),
				WriteResponseMetadata{
					Diff:      diff,
					Additions: additions,
					Removals:  removals,
				},
			), nil
		})
}

func normalizeWriteMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "overwrite":
		return "overwrite"
	case "append":
		return "append"
	default:
		return ""
	}
}

func writeModePastTense(mode string) string {
	if mode == "append" {
		return "appended"
	}
	return "written"
}
