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
  KeyOutlined,
  MessageOutlined,
  PlusSquareOutlined,
  SearchOutlined,
  SettingOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import type { WorkbenchMode, WorkbenchViewModel } from './workbenchTypes.ts';

const projects = [
  {
    id: 'agent-builder',
    name: 'agent-builder',
    path: 'C:\\Users\\ytq\\work\\ai\\agent-builder',
    isGitRepository: true,
    branch: 'main',
    current: true,
  },
];

const sidebarActions = [
  { id: 'new-chat', label: '新对话', icon: <PlusSquareOutlined /> },
  { id: 'search', label: '搜索', icon: <SearchOutlined /> },
  { id: 'skills', label: '技能', icon: <ToolOutlined /> },
  { id: 'plugins', label: '插件', icon: <AppstoreAddOutlined /> },
  { id: 'automations', label: '自动化', icon: <SettingOutlined /> },
];

const settings = {
  activeKey: 'general',
  navItems: [
    { key: 'general', label: '常规', icon: <SettingOutlined /> },
    { key: 'providers', label: '服务商', icon: <CloudServerOutlined /> },
    { key: 'permissions', label: '权限', icon: <KeyOutlined /> },
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
  permissions: [
    {
      key: 'workspace',
      title: '默认权限',
      description: '默认情况下，Agent Builder 可以读取并编辑其工作区中的文件。',
      enabled: true,
    },
    {
      key: 'review',
      title: '自动审核',
      description: '自动审核额外访问权限请求，并在高风险操作前保留确认步骤。',
      enabled: true,
    },
    {
      key: 'full-access',
      title: '完全访问权限',
      description: '允许在明确授权后访问更宽的文件范围并运行联网命令。',
      enabled: true,
    },
  ],
  defaultEditor: 'vscode',
  terminalProfile: 'powershell',
  editorOptions: [
    { label: 'VS Code', value: 'vscode' },
    { label: 'Visual Studio', value: 'visual-studio' },
    { label: '系统默认', value: 'system' },
  ],
  terminalOptions: [
    { label: 'PowerShell', value: 'powershell' },
    { label: 'Windows Terminal', value: 'windows-terminal' },
    { label: '命令提示符', value: 'cmd' },
  ],
};

export function getInitialWorkbenchViewModel(mode: WorkbenchMode = 'project'): WorkbenchViewModel {
  const currentProject = projects.find((project) => project.current) ?? projects[0];

  return {
    mode,
    currentProject,
    projects,
    sessions: [],
    sidebarActions,
    composer: {
      placeholder: '要求后续变更',
      permissionLabel: '完全访问权限',
      modelLabel: 'custom · 5.5 中',
    },
    settings,
  };
}

export const settingAction = { id: 'settings', label: '设置', icon: <SettingOutlined /> };
export const projectGroupIcon = <FolderOutlined />;
