import assert from 'node:assert/strict';

const values = new Map();
globalThis.window = {
  localStorage: {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  },
  setTimeout,
  clearTimeout,
};

const preferences = await import('../src/runtime/sidebarPreferences.ts');
assert.deepEqual(preferences.loadSidebarPreferences(), {
  projectsOpen: true,
  sessionsOpen: true,
  sidebarCollapsed: false,
  projectOverrides: {},
});

preferences.saveSidebarGroupPreference('projects', false);
preferences.saveSidebarCollapsedPreference(true);
for (let index = 0; index < 5_010; index += 1) {
  preferences.saveProjectExpandedPreference(`project-${index}`, index % 2 === 0);
}
let snapshot = preferences.loadSidebarPreferences();
assert.equal(snapshot.projectsOpen, false);
assert.equal(snapshot.sidebarCollapsed, true);
assert.equal(Object.keys(snapshot.projectOverrides).length, 5_000, 'project preferences stay bounded');
assert.equal(snapshot.projectOverrides['project-0'], undefined, 'oldest project preference is evicted');

preferences.reconcileProjectExpandedPreferences(['project-5009']);
snapshot = preferences.loadSidebarPreferences();
assert.deepEqual(Object.keys(snapshot.projectOverrides), ['project-5009'], 'deleted projects are reconciled');

console.log('sidebar preferences smoke passed');
