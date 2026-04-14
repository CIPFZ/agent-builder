package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

type Context struct {
	Root  string
	Files []File
}

type File struct {
	Name    string
	Content string
}

type Loader struct {
	root string
}

func NewLoader(root string) *Loader {
	return &Loader{root: root}
}

func (l *Loader) Load() (Context, error) {
	ctx := Context{
		Root: l.root,
	}

	for _, name := range []string{"CLAUDE.md", "AGENTS.md", "SOUL.md", "TOOLS.md"} {
		path := filepath.Join(l.root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Context{}, err
		}

		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}

		ctx.Files = append(ctx.Files, File{
			Name:    name,
			Content: content,
		})
	}

	return ctx, nil
}
