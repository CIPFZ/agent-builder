// Workarounds for WebView2 cursor stickiness on Windows.
//
// WebView2 does not reliably propagate every CSS cursor transition back to
// the native Win32 cursor (see MicrosoftEdge/WebView2Feedback#2766). The
// visible symptom: hover a panel-resize gutter (or the native window border),
// then move back into the app — the arrow keeps showing the resize cursor
// until something else forces a cursor update.
//
// `nudgeCursorRecompute` forces that update by pinning an explicit cursor on
// <body> for one frame and then clearing it: the style change makes Chromium
// re-resolve the effective cursor under the pointer and push it to the OS.

let nudgeFrame: number | undefined;

export function nudgeCursorRecompute(): void {
  if (typeof document === 'undefined') {
    return;
  }
  document.body.style.cursor = 'default';
  if (nudgeFrame !== undefined) {
    window.cancelAnimationFrame(nudgeFrame);
  }
  nudgeFrame = window.requestAnimationFrame(() => {
    nudgeFrame = undefined;
    document.body.style.cursor = '';
  });
}

let installed = false;

// Entering the webview from outside (native title bar, window resize border,
// another window) is exactly when a stale OS cursor can survive; refresh it
// on every re-entry. Idempotent — safe to call from any component setup.
export function installWebviewCursorRecovery(): void {
  if (installed || typeof document === 'undefined') {
    return;
  }
  installed = true;
  document.documentElement.addEventListener('mouseenter', () => {
    nudgeCursorRecompute();
  });
}
