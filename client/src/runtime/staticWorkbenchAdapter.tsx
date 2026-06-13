import {
  AppstoreAddOutlined,
  BarChartOutlined,
  CloudServerOutlined,
  CodeOutlined,
  ControlOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  ExperimentOutlined,
  FolderOutlined,
  GlobalOutlined,
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
  activeKey: 'general',
  navItems: [
    { key: 'general', label: '常规', icon: <SettingOutlined /> },
    { key: 'providers', label: '服务商', icon: <CloudServerOutlined /> },
    { key: 'common', label: '通用', icon: <ControlOutlined /> },
    { key: 'h5', label: 'H5 访问', icon: <GlobalOutlined /> },
    { key: 'im', label: 'IM 接入', icon: <MessageOutlined /> },
    { key: 'terminal', label: '终端', icon: <CodeOutlined /> },
    { key: 'mcp', label: 'MCP', icon: <DatabaseOutlined /> },
    { key: 'agents', label: 'Agents', icon: <DesktopOutlined /> },
    { key: 'skills', label: '技能', icon: <ToolOutlined /> },
    { key: 'memory', label: '记忆', icon: <HistoryOutlined /> },
    { key: 'plugins', label: '插件', icon: <AppstoreAddOutlined /> },
    { key: 'computer-use', label: 'Computer Use', icon: <DesktopOutlined /> },
    { key: 'token-usage', label: 'Token 用量', icon: <BarChartOutlined /> },
    { key: 'diagnostics', label: '诊断', icon: <ExperimentOutlined /> },
  ],
  permissions: [],
  permissionOptions: [],
  defaultEditor: '',
  terminalProfile: '',
  editorOptions: [],
  terminalOptions: [],
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
    sidebarActions,
    conversation: [],
    timeline: [],
    turnDiagnostics: undefined,
    interruptedTurn: undefined,
    runProjection: undefined,
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
  async createSession() {
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
  async markInterruptedDone() {
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
  async testConfiguredProvider() {
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
};

export const settingAction = { id: 'settings', label: '设置', icon: <SettingOutlined /> };
export const projectGroupIcon = <FolderOutlined />;
