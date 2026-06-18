import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const adapterPath = resolve(repoRoot, 'client', 'src', 'runtime', 'wailsWorkbenchAdapter.ts');
const typesPath = resolve(repoRoot, 'client', 'src', 'runtime', 'workbenchTypes.ts');
const inspectorPath = resolve(repoRoot, 'client', 'src', 'features', 'diagnostics', 'ReactCallchainInspector.tsx');
const permissionPath = resolve(repoRoot, 'client', 'src', 'features', 'permissions', 'PermissionGate.tsx');

const [adapterSource, typesSource, inspectorSource, permissionSource] = await Promise.all([
  readFile(adapterPath, 'utf8'),
  readFile(typesPath, 'utf8'),
  readFile(inspectorPath, 'utf8'),
  readFile(permissionPath, 'utf8'),
]);

assert.match(typesSource, /export interface ToolResultDeliveryViewModel/);
assert.match(typesSource, /stopReasonMessage\?: string/);
assert.match(typesSource, /deliveredToolResultCount\?: number/);
assert.match(typesSource, /undeliveredToolResultCount\?: number/);

assert.match(adapterSource, /interface RuntimeToolResultDeliveryDTO/);
assert.match(adapterSource, /function mapToolResultDeliveries/);
assert.match(adapterSource, /summaryDeliveries = mapToolResultDeliveries\(summary\.toolResultDeliveries\)/);
assert.match(adapterSource, /toolResultDeliveries,/);
assert.doesNotMatch(adapterSource, /completed_without_final_assistant/);

assert.match(inspectorSource, /summary\.stopReasonMessage \|\| summary\.stopReason \|\| 'running'/);
assert.match(inspectorSource, /fed back to model/);
assert.match(inspectorSource, /persisted output/);
assert.match(inspectorSource, /node\.evidence\?\.deliveredToModel/);
assert.match(inspectorSource, /node\.evidence\?\.deliveryReason/);
assert.doesNotMatch(inspectorSource, /turn_completed_without_final_assistant/);

assert.match(permissionSource, /type PermissionDecision = 'allow' \| 'allow_session' \| 'deny'/);
assert.match(permissionSource, /decide\('allow'\)/);
assert.match(permissionSource, /decide\('allow_session'\)/);
assert.match(permissionSource, /decide\('deny'\)/);

console.log('phase03 tool loop diagnostics smoke passed');
