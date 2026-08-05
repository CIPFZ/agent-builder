import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const clientRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));
const repoRoot = resolve(clientRoot, '..');
const shell = readFileSync(resolve(clientRoot, 'src/app/shell/WorkbenchShell.tsx'), 'utf8');
const adapter = readFileSync(resolve(clientRoot, 'src/runtime/wailsWorkbenchAdapter.ts'), 'utf8');
const bridge = readFileSync(resolve(repoRoot, 'desktop/runtime_bridge.go'), 'utf8');
const processMemory = readFileSync(resolve(repoRoot, 'desktop/process_memory_windows.go'), 'utf8');
const runtimeGuard = readFileSync(resolve(repoRoot, 'internal/runtime/runtime_idle_memory_guard.go'), 'utf8');

assertIncludes(shell, 'IDLE_MEMORY_GUARD_MINIMUM_MS = 2 * 60 * 1000', 'two-minute client idle floor');
assertIncludes(shell, 'response.memorySupported', 'trusted platform measurement requirement');
assertIncludes(shell, 'response.sustained', 'sustained high-water requirement');
assertIncludes(shell, 'hasUnsavedComposerDraft()', 'unsaved draft protection');
assertIncludes(shell, 'hasActiveOverlay()', 'overlay protection');
assertIncludes(shell, 'hasTerminalInteraction()', 'terminal interaction protection');
assertIncludes(shell, 'window.sessionStorage.setItem(IDLE_MEMORY_GUARD_UI_STATE_KEY', 'pure UI recovery snapshot');
assertIncludes(shell, 'await reloadWindow()', 'controlled Wails reload');
assertNotIncludes(shell, 'window.gc(', 'production forced GC');
assertNotIncludes(shell, 'performance.memory', 'renderer memory guess in React');

assertIncludes(adapter, "const wailsRuntimePath = '/wails/runtime.js'", 'Wails runtime transport');
assertIncludes(adapter, 'bridge.IdleMemoryGuard(request)', 'generated Wails guard binding');
assertIncludes(adapter, 'runtime.Window.Reload()', 'Wails window reload');
assertNotIncludes(adapter, 'fetch(', 'HTTP fallback');

assertIncludes(bridge, 'desktopMemoryGuardTreeHighWaterBytes', 'whole-process-tree threshold');
assertIncludes(bridge, 'desktopMemoryGuardWebViewHighWaterBytes', 'WebView subtree threshold');
assertIncludes(bridge, 'desktopMemoryGuardRequiredSamples', 'sustained sample counter');
assertIncludes(processMemory, 'windows.CreateToolhelp32Snapshot', 'Windows descendant snapshot');
assertIncludes(processMemory, 'GetProcessMemoryInfo', 'trusted Private Memory read');
assertIncludes(runtimeGuard, 'pendingPermissions > 0', 'Runtime permission authority');
assertIncludes(runtimeGuard, 'status.ResourceGovernor.Resources', 'Resource Governor authority');

console.log('Idle memory guard contract smoke passed.');

function assertIncludes(source, expected, label) {
  if (!source.includes(expected)) throw new Error(`missing ${label}: ${expected}`);
}

function assertNotIncludes(source, unexpected, label) {
  if (source.includes(unexpected)) throw new Error(`unexpected ${label}: ${unexpected}`);
}
