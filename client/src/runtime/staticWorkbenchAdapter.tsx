import {
  AppstoreAddOutlined,
  BarChartOutlined,
  BranchesOutlined,
  CloudServerOutlined,
  CodeOutlined,
  ControlOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  ExperimentOutlined,
  FolderOutlined,
  HistoryOutlined,
  MessageOutlined,
  PlusSquareOutlined,
  SearchOutlined,
  SettingOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import type { WorkbenchAdapter, WorkbenchMode, WorkbenchViewModel } from './workbenchTypes.ts';

const sidebarActions = [
  { id: 'new-chat', label: '新对话', icon: <PlusSquareOutlined /> },
  { id: 'search', label: '搜索', icon: <SearchOutlined /> },
  { id: 'plugins', label: '插件', icon: <AppstoreAddOutlined /> },
  { id: 'automations', label: '自动化', icon: <SettingOutlined /> },
];

const settings = {
  activeKey: 'common',
  navItems: [
    { key: 'common', label: '通用', icon: <ControlOutlined /> },
    { key: 'providers', label: '服务商', icon: <CloudServerOutlined /> },
    { key: 'im', label: 'IM 接入', icon: <MessageOutlined /> },
    { key: 'terminal', label: '终端', icon: <CodeOutlined /> },
    { key: 'mcp', label: 'MCP', icon: <DatabaseOutlined /> },
    { key: 'hooks', label: 'Hooks', icon: <BranchesOutlined /> },
    { key: 'agents', label: 'Agents', icon: <DesktopOutlined /> },
    { key: 'skills', label: '技能', icon: <ToolOutlined /> },
    { key: 'memory', label: '记忆', icon: <HistoryOutlined /> },
    { key: 'plugins', label: '插件', icon: <AppstoreAddOutlined /> },
    { key: 'computer-use', label: 'Computer Use', icon: <DesktopOutlined /> },
    { key: 'context', label: '上下文', icon: <BarChartOutlined /> },
    { key: 'diagnostics', label: '诊断', icon: <ExperimentOutlined /> },
  ],
  permissions: [],
  permissionOptions: [],
  defaultEditor: 'system',
  terminalProfile: '',
  editorOptions: [{ value: 'system', label: '系统文件管理器' }],
  terminalOptions: [],
  appearance: { colorMode: 'system' as const, themeId: 'builtin.default' },
  providerTypes: [],
  providers: [],
  configuredProviders: [],
  plugins: [],
  skills: [],
  mcpServers: [],
  mcpToolsByServer: {},
  mcpResourcesByServer: {},
  mcpPromptsByServer: {},
};

export function getInitialWorkbenchViewModel(mode: WorkbenchMode = 'project'): WorkbenchViewModel {
  return {
    mode,
    currentProject: {
      id: '',
      name: '',
      path: '',
      isGitRepository: false,
    },
    projects: [],
    sessions: [],
    conversationTarget: { kind: 'draft', scope: 'standalone' },
    sidebarActions,
    conversation: [],
    turnDiagnostics: undefined,
    runProjection: undefined,
    agentTasks: [],
    agentRoles: [],
    reactCallchain: undefined,
    contextDiagnostics: undefined,
    recovery: {
      activeTurns: [],
      interruptedTurns: [],
      recoverableErrors: [],
      pendingPermissions: [],
      pendingMCPRequests: [],
      compactBoundaries: [],
      actions: [],
    },
    hooks: [],
    hookExecutions: {
      items: [],
      total: 0,
      started: 0,
      completed: 0,
      blocked: 0,
      failed: 0,
      skipped: 0,
      rewritten: 0,
      contextInjected: 0,
    },
    pendingPermissions: [],
    composer: {
      placeholder: '请输入任务',
      permissionLabel: '未配置权限',
      modelLabel: '未配置模型',
      permissionOptions: [],
      modelOptions: [],
      capabilityLabel: '0 skills / 0 MCP',
      busy: false,
    },
    settings,
  };
}

async function runtimeUnavailable(): Promise<never> {
  throw new Error('runtime unavailable');
}

export const staticWorkbenchAdapter: WorkbenchAdapter = {
  async loadInitialViewModel(mode = 'project') {
    return getInitialWorkbenchViewModel(mode);
  },
  async refresh(current) {
    return current;
  },
  async openProject() {
    return runtimeUnavailable();
  },
  async createProject() {
    return runtimeUnavailable();
  },
  async renameProject() {
    return runtimeUnavailable();
  },
  async openProjectInExplorer() {
    return runtimeUnavailable();
  },
  async removeProject() {
    return runtimeUnavailable();
  },
  async selectProjectDirectory() {
    return runtimeUnavailable();
  },
  async selectSession() {
    return runtimeUnavailable();
  },
  async renameSession() {
    return runtimeUnavailable();
  },
  async deleteSession() {
    return runtimeUnavailable();
  },
  async selectModel() {
    return runtimeUnavailable();
  },
  async selectPermissionMode() {
    return runtimeUnavailable();
  },
  async decidePermission() {
    return runtimeUnavailable();
  },
  async sendPrompt() {
    return runtimeUnavailable();
  },
  async cancelTurn() {
    return runtimeUnavailable();
  },
  async resumeInterruptedTurn() {
    return runtimeUnavailable();
  },
  async markInterruptedDone() {
    return runtimeUnavailable();
  },
  async discardInterruptedTurn() {
    return runtimeUnavailable();
  },
  async retryRecoverableError() {
    return runtimeUnavailable();
  },
  async manualCompact() {
    return runtimeUnavailable();
  },
  async manualSnip() {
    return runtimeUnavailable();
  },
  async resumeRunCheckpoint() {
    return runtimeUnavailable();
  },
  async readRunSchedulerPlan() {
    return runtimeUnavailable();
  },
  async executeRunTask() {
    return runtimeUnavailable();
  },
  async sendAgentTaskFollowUp() {
    return runtimeUnavailable();
  },
  async cancelAgentTask() {
    return runtimeUnavailable();
  },
  async listSessionTerminals() {
    return runtimeUnavailable();
  },
  async createTerminal() {
    return runtimeUnavailable();
  },
  async writeTerminalInput() {
    return runtimeUnavailable();
  },
  async resizeTerminal() {
    return runtimeUnavailable();
  },
  async subscribeTerminalEvents() {
    return runtimeUnavailable();
  },
  async deleteTerminal() {
    return runtimeUnavailable();
  },
  async selectTerminalProfile() {
    return runtimeUnavailable();
  },
  async selectAppearance(current, appearance) {
    return { ...current, settings: { ...current.settings, appearance } };
  },
  async selectOpenTarget(current, targetID) {
    return { ...current, settings: { ...current.settings, defaultEditor: targetID } };
  },
  async saveConfiguredProvider() {
    return runtimeUnavailable();
  },
  async deleteConfiguredProvider() {
    return runtimeUnavailable();
  },
  async discoverProviderDraftModels() {
    return runtimeUnavailable();
  },
  async discoverConfiguredProviderModels() {
    return runtimeUnavailable();
  },
  async testProviderDraft() {
    return runtimeUnavailable();
  },
  async testConfiguredProvider() {
    return runtimeUnavailable();
  },
  async measureProviderDraftLatency() {
    return runtimeUnavailable();
  },
  async measureConfiguredProviderLatency() {
    return runtimeUnavailable();
  },
  async refreshSkills() {
    return runtimeUnavailable();
  },
  async setSkillEnabled() {
    return runtimeUnavailable();
  },
  async refreshMCPServer() {
    return runtimeUnavailable();
  },
  async saveMCPServer() {
    return runtimeUnavailable();
  },
  async setMCPServerEnabled() {
    return runtimeUnavailable();
  },
  async setMCPToolEnabled() {
    return runtimeUnavailable();
  },
  async loadMCPServerDetails() {
    return runtimeUnavailable();
  },
  async listProjectMemories() {
    return runtimeUnavailable();
  },
  async getProjectMemory() {
    return runtimeUnavailable();
  },
  async createProjectMemory() {
    return runtimeUnavailable();
  },
  async updateProjectMemory() {
    return runtimeUnavailable();
  },
  async setProjectMemoryEnabled() {
    return runtimeUnavailable();
  },
  async deleteProjectMemory() {
    return runtimeUnavailable();
  },
  async refreshProjectMemoryIndex() {
    return runtimeUnavailable();
  },
  async getContextGovernanceSettings() {
    return runtimeUnavailable();
  },
  async saveContextGovernanceSettings() {
    return runtimeUnavailable();
  },
};

export const settingAction = { id: 'settings', label: '设置', icon: <SettingOutlined /> };
export const projectGroupIcon = <FolderOutlined />;
