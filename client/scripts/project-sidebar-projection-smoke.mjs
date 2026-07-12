import { readFileSync } from 'node:fs';
import { join } from 'node:path';

const root = process.cwd();
const adapter = readFileSync(join(root, 'src/runtime/wailsWorkbenchAdapter.ts'), 'utf8');
const sidebar = readFileSync(join(root, 'src/features/sidebar/Sidebar.tsx'), 'utf8');

const checks = [
  {
    name: 'hydrate does not derive projects from currentProject singleton',
    ok: !adapter.includes('projects: currentProject.path ? [currentProject] : []'),
  },
  {
    name: 'hydrate reads runtime sidebar projection',
    ok: adapter.includes('SidebarProjection') && adapter.includes('RuntimeSidebarProjectionResponseDTO'),
  },
  {
    name: 'session mapping does not fill projectId from current project',
    ok: !adapter.includes('session.projectId || currentProjectID'),
  },
  {
    name: 'sidebar keeps standalone sessions separate',
    ok: sidebar.includes("session.scope === 'standalone'"),
  },
  {
    name: 'sidebar keeps project sessions under matching project id',
    ok: sidebar.includes("session.scope === 'project'") && sidebar.includes('session.projectId === project.id'),
  },
  {
    name: 'new chat is a local draft and does not mutate runtime session state',
    ok: !adapter.includes('startConversationDraft') && !adapter.includes("bridge.NewChat('')") && !adapter.includes('await bridge.CreateSession'),
  },
];

const failed = checks.filter((check) => !check.ok);
for (const check of checks) {
  console.log(`${check.ok ? 'PASS' : 'FAIL'} ${check.name}`);
}
if (failed.length > 0) {
  process.exitCode = 1;
}
