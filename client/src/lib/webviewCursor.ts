// Refreshes WebView2 cursor state after crossing native/WebView boundaries or
// after one of our internal column splitters finishes interacting.

const cursorRecoveryAttribute = 'data-webview-cursor-recovery';
const cursorRecoveryStyleID = 'webview-cursor-recovery-style';

let nudgeFrame: number | undefined;
let installed = false;

function ensureCursorRecoveryStyle(): void {
  if (document.getElementById(cursorRecoveryStyleID)) {
    return;
  }

  const style = document.createElement('style');
  style.id = cursorRecoveryStyleID;
  style.textContent = `
html[${cursorRecoveryAttribute}],
html[${cursorRecoveryAttribute}] * {
  cursor: default !important;
}
`;
  (document.head || document.documentElement).appendChild(style);
}

export function nudgeCursorRecompute(): void {
  if (typeof document === 'undefined') {
    return;
  }

  ensureCursorRecoveryStyle();
  document.documentElement.setAttribute(cursorRecoveryAttribute, '');

  if (nudgeFrame !== undefined) {
    window.cancelAnimationFrame(nudgeFrame);
  }

  nudgeFrame = window.requestAnimationFrame(() => {
    nudgeFrame = window.requestAnimationFrame(() => {
      nudgeFrame = undefined;
      document.documentElement.removeAttribute(cursorRecoveryAttribute);
    });
  });
}

// Idempotent, safe to call from component setup.
export function installWebviewCursorRecovery(): void {
  if (installed || typeof document === 'undefined') {
    return;
  }

  installed = true;
  document.documentElement.addEventListener('mouseenter', nudgeCursorRecompute);
  document.documentElement.addEventListener('pointerenter', nudgeCursorRecompute);
  window.addEventListener('focus', nudgeCursorRecompute);
}
