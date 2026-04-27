package app

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const operatorRoute = "/operator/"

//go:embed operator_dist/* operator_dist/assets/*
var embeddedOperatorDist embed.FS

var operatorDistCandidates = []string{
	filepath.Join("web", "operator", "dist"),
	filepath.Join("..", "web", "operator", "dist"),
	filepath.Join("..", "..", "web", "operator", "dist"),
}

func handleOperatorUI() http.Handler {
	distDir, ok := findOperatorDistDir()
	if !ok {
		if embeddedDist, embeddedOK := embeddedOperatorDistFS(); embeddedOK {
			return handleOperatorFileSystem(http.FS(embeddedDist))
		}
		return handleMissingOperatorUI()
	}
	return handleOperatorFileSystem(http.Dir(distDir))
}

func handleOperatorFileSystem(fileSystem http.FileSystem) http.Handler {
	fileServer := http.FileServer(fileSystem)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/operator" {
			http.Redirect(w, r, operatorRoute, http.StatusMovedPermanently)
			return
		}
		if !strings.HasPrefix(r.URL.Path, operatorRoute) {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix(operatorRoute, fileServer).ServeHTTP(w, r)
	})
}

func handleMissingOperatorUI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, operatorMissingHTML())
	})
}

func findOperatorDistDir() (string, bool) {
	if configured := strings.TrimSpace(os.Getenv("MYCLAW_OPERATOR_DIST")); configured != "" {
		return validOperatorDistDir(configured)
	}

	candidates := append([]string(nil), operatorDistCandidates...)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "web", "operator", "dist"),
			filepath.Join(exeDir, "operator"),
		)
	}

	for _, candidate := range candidates {
		if dir, ok := validOperatorDistDir(candidate); ok {
			return dir, true
		}
	}
	return "", false
}

func embeddedOperatorDistFS() (fs.FS, bool) {
	dist, err := fs.Sub(embeddedOperatorDist, "operator_dist")
	if err != nil {
		return nil, false
	}
	info, err := fs.Stat(dist, "index.html")
	if err != nil || info.IsDir() {
		return nil, false
	}
	return dist, true
}

func validOperatorDistDir(path string) (string, bool) {
	clean := filepath.Clean(path)
	indexPath := filepath.Join(clean, "index.html")
	info, err := os.Stat(indexPath)
	if err != nil || info.IsDir() {
		return "", false
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return clean, true
	}
	return abs, true
}

func operatorMissingHTML() string {
	return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <title>myclaw Operator UI not built</title>
  <style>
    body{font-family:Segoe UI,Arial,sans-serif;margin:48px;background:#f7f4ef;color:#1f1d1a}
    main{max-width:760px;margin:auto;background:#fff;border:1px solid #e7dfd2;border-radius:20px;padding:28px;box-shadow:0 20px 60px rgba(44,35,24,.08)}
    code{background:#f3eee6;border-radius:8px;padding:2px 6px}
  </style>
</head>
<body>
  <main>
    <h1>Operator UI has not been built</h1>
    <p>Run <code>npm install</code> and <code>npm run build</code> in <code>web/operator</code>, then restart <code>myclawd</code>.</p>
    <p>For development, run <code>scripts/start-operator.ps1</code> from the repository root.</p>
  </main>
</body>
</html>`
}
