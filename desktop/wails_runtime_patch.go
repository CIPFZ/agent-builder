package main

import (
	_ "embed"
	"net/http"
)

// This is Wails' bundled runtime with its WebView-edge resize hit area limited
// to frameless windows. Agent Builder uses a normal framed window, so window
// resizing must stay in the native frame instead of the React content area.
//
//go:embed runtime/wails_runtime.js
var patchedWailsRuntimeJS []byte

func patchedWailsRuntimeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/wails/runtime.js" {
			response.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			response.Header().Set("Cache-Control", "no-cache")
			_, _ = response.Write(patchedWailsRuntimeJS)
			return
		}

		next.ServeHTTP(response, request)
	})
}
