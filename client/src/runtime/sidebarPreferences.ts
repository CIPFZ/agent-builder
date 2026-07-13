const STORAGE_KEY = 'agent-builder.sidebar-preferences';
const STORAGE_VERSION = 1;
const MAX_PROJECT_OVERRIDES = 5_000;
const WRITE_DELAY_MS = 150;

interface StoredSidebarPreferences {
  version: typeof STORAGE_VERSION;
  projectsOpen: boolean;
  sessionsOpen: boolean;
  sidebarCollapsed: boolean;
  // Insertion order is recency order. Re-toggling moves an entry to the end.
  projectOverrides: Array<[projectId: string, expanded: 0 | 1]>;
}

export interface SidebarPreferencesSnapshot {
  projectsOpen: boolean;
  sessionsOpen: boolean;
  sidebarCollapsed: boolean;
  projectOverrides: Record<string, boolean>;
}

const defaults = (): StoredSidebarPreferences => ({
  version: STORAGE_VERSION,
  projectsOpen: true,
  sessionsOpen: true,
  sidebarCollapsed: false,
  projectOverrides: [],
});

let cached: StoredSidebarPreferences | undefined;
let writeTimer: ReturnType<typeof setTimeout> | undefined;

function storageAvailable() {
  return typeof window !== 'undefined' && Boolean(window.localStorage);
}

function readStored(): StoredSidebarPreferences {
  if (cached) return cached;
  const fallback = defaults();
  if (!storageAvailable()) return (cached = fallback);
  try {
    const parsed = JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? 'null') as Partial<StoredSidebarPreferences> | null;
    if (!parsed || parsed.version !== STORAGE_VERSION || !Array.isArray(parsed.projectOverrides)) return (cached = fallback);
    const seen = new Set<string>();
    const projectOverrides: StoredSidebarPreferences['projectOverrides'] = [];
    for (const entry of parsed.projectOverrides.slice(-MAX_PROJECT_OVERRIDES)) {
      if (!Array.isArray(entry) || typeof entry[0] !== 'string' || !entry[0] || (entry[1] !== 0 && entry[1] !== 1) || seen.has(entry[0])) continue;
      seen.add(entry[0]);
      projectOverrides.push([entry[0], entry[1]]);
    }
    return (cached = {
      version: STORAGE_VERSION,
      projectsOpen: typeof parsed.projectsOpen === 'boolean' ? parsed.projectsOpen : true,
      sessionsOpen: typeof parsed.sessionsOpen === 'boolean' ? parsed.sessionsOpen : true,
      sidebarCollapsed: typeof parsed.sidebarCollapsed === 'boolean' ? parsed.sidebarCollapsed : false,
      projectOverrides,
    });
  } catch {
    return (cached = fallback);
  }
}

function scheduleWrite() {
  if (!storageAvailable()) return;
  if (writeTimer) window.clearTimeout(writeTimer);
  writeTimer = window.setTimeout(() => {
    writeTimer = undefined;
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(readStored()));
    } catch {
      // UI preferences are best-effort and must never block the workbench.
    }
  }, WRITE_DELAY_MS);
}

export function loadSidebarPreferences(): SidebarPreferencesSnapshot {
  const stored = readStored();
  return {
    projectsOpen: stored.projectsOpen,
    sessionsOpen: stored.sessionsOpen,
    sidebarCollapsed: stored.sidebarCollapsed,
    projectOverrides: Object.fromEntries(stored.projectOverrides.map(([id, value]) => [id, value === 1])),
  };
}

export function saveSidebarGroupPreference(group: 'projects' | 'sessions', open: boolean) {
  const stored = readStored();
  if (group === 'projects') stored.projectsOpen = open;
  else stored.sessionsOpen = open;
  scheduleWrite();
}

export function saveSidebarCollapsedPreference(collapsed: boolean) {
  readStored().sidebarCollapsed = collapsed;
  scheduleWrite();
}

export function saveProjectExpandedPreference(projectId: string, expanded: boolean) {
  const stored = readStored();
  stored.projectOverrides = stored.projectOverrides.filter(([id]) => id !== projectId);
  stored.projectOverrides.push([projectId, expanded ? 1 : 0]);
  if (stored.projectOverrides.length > MAX_PROJECT_OVERRIDES) {
    stored.projectOverrides.splice(0, stored.projectOverrides.length - MAX_PROJECT_OVERRIDES);
  }
  scheduleWrite();
}

export function reconcileProjectExpandedPreferences(projectIds: readonly string[]) {
  const stored = readStored();
  const valid = new Set(projectIds);
  const next = stored.projectOverrides.filter(([id]) => valid.has(id));
  if (next.length === stored.projectOverrides.length) return;
  stored.projectOverrides = next;
  scheduleWrite();
}
