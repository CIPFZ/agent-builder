import { useEffect, useMemo, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import {
  ArrowLeftOutlined,
  ApiOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeInvisibleOutlined,
  EyeTwoTone,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Checkbox,
  Collapse,
  ConfigProvider,
  Flex,
  Form,
  Input,
  InputNumber,
  Layout,
  List,
  Menu,
  message,
  Modal,
  Select,
  Steps,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { MenuProps } from 'antd';
import type {
  ConfiguredProviderViewModel,
  ContextGovernanceSettingsViewModel,
  ContextUsageViewModel,
  HookExecutionSummaryViewModel,
  HookViewModel,
  ProjectMemoryCreateViewModel,
  ProjectMemoryRecordViewModel,
  ProjectMemoryUpdateViewModel,
  ProjectViewModel,
  ProviderDraftDiscoveryRequestViewModel,
  ProviderCatalogItemViewModel,
  ProviderModelDiscoveryViewModel,
  ProviderModelViewModel,
  ProviderTestViewModel,
  RuntimeMCPServerViewModel,
  RuntimeModelOptionViewModel,
  SettingsViewModel,
  WorkbenchMode,
} from '../../runtime/workbenchTypes.ts';
import { HookSettingsPanel } from '../hooks/HookSettingsPanel.tsx';
import { nudgeCursorRecompute } from '../../lib/webviewCursor.ts';
import styles from './SettingsPanel.module.css';
import type { AppearanceSettings, ColorMode } from '../../theme/contract.ts';

const { Content, Sider } = Layout;
const { Paragraph, Text, Title } = Typography;

type ProviderFormValues = Omit<ConfiguredProviderViewModel, 'defaultModel'> & {
  defaultModel?: string | string[];
  token?: string;
};

type MCPServerFormValues = Omit<RuntimeMCPServerViewModel, 'args' | 'counts' | 'disabled' | 'enabledTools' | 'disabledTools'> & {
  args?: string;
  enabled?: boolean;
};

const protocolOptions = [
  { label: 'OpenAI 兼容', value: 'openai-compat' },
  { label: 'Anthropic', value: 'anthropic' },
];

interface SettingsPanelProps {
  settings: SettingsViewModel;
  project?: ProjectViewModel;
  hooks?: HookViewModel[];
  hookExecutions?: HookExecutionSummaryViewModel;
  onModeChange: (mode: WorkbenchMode) => void;
  onSettingsRefresh: () => Promise<SettingsViewModel>;
  onProviderSave: (provider: ConfiguredProviderViewModel & { token?: string }) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDelete: (providerID: string) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDiscoverDraftModels: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderModelDiscoveryViewModel>;
  onProviderDiscoverModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  onProviderDraftTest: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  onProviderTest: (providerID: string) => Promise<ProviderTestViewModel>;
  onProviderDraftLatency: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  onProviderLatency: (providerID: string) => Promise<ProviderTestViewModel>;
  selectedModel?: RuntimeModelOptionViewModel;
  contextUsage?: ContextUsageViewModel;
  onModelSelect: (configuredProviderID: string, model: string) => Promise<void>;
  onTerminalProfileSelect: (profileID: string) => Promise<SettingsViewModel>;
  onAppearanceSelect: (appearance: AppearanceSettings) => Promise<SettingsViewModel>;
  onOpenTargetSelect: (targetID: string) => Promise<SettingsViewModel>;
  onContextGovernanceLoad: () => Promise<ContextGovernanceSettingsViewModel>;
  onContextGovernanceSave: (settings: ContextGovernanceSettingsViewModel) => Promise<ContextGovernanceSettingsViewModel>;
  onSkillRefresh: () => Promise<SettingsViewModel>;
  onSkillToggle: (name: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPServerRefresh: (name: string) => Promise<SettingsViewModel>;
  onMCPServerSave: (server: RuntimeMCPServerViewModel) => Promise<SettingsViewModel>;
  onMCPServerToggle: (name: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPToolToggle: (server: string, tool: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPServerDetailsLoad: (name: string) => Promise<SettingsViewModel>;
  onProjectMemoryList: (projectID: string) => Promise<{ records: ProjectMemoryRecordViewModel[]; root?: string }>;
  onProjectMemoryDetail: (memoryID: string) => Promise<ProjectMemoryRecordViewModel>;
  onProjectMemoryCreate: (request: ProjectMemoryCreateViewModel) => Promise<ProjectMemoryRecordViewModel>;
  onProjectMemoryUpdate: (memoryID: string, request: ProjectMemoryUpdateViewModel) => Promise<ProjectMemoryRecordViewModel>;
  onProjectMemoryEnabledChange: (memoryID: string, enabled: boolean) => Promise<ProjectMemoryRecordViewModel>;
  onProjectMemoryDelete: (memoryID: string, reason?: string) => Promise<ProjectMemoryRecordViewModel>;
  onProjectMemoryRefresh: (projectID: string) => Promise<unknown>;
}

export function SettingsPanel({
  settings,
  project,
  hooks,
  hookExecutions,
  onModeChange,
  onSettingsRefresh,
  onProviderSave,
  onProviderDelete,
  onProviderDiscoverDraftModels,
  onProviderDiscoverModels,
  onProviderDraftTest,
  onProviderTest,
  onProviderDraftLatency,
  onProviderLatency,
  selectedModel,
  contextUsage,
  onModelSelect,
  onTerminalProfileSelect,
  onAppearanceSelect,
  onOpenTargetSelect,
  onContextGovernanceLoad,
  onContextGovernanceSave,
  onSkillRefresh,
  onSkillToggle,
  onMCPServerRefresh,
  onMCPServerSave,
  onMCPServerToggle,
  onMCPToolToggle,
  onMCPServerDetailsLoad,
  onProjectMemoryList,
  onProjectMemoryDetail,
  onProjectMemoryCreate,
  onProjectMemoryUpdate,
  onProjectMemoryEnabledChange,
  onProjectMemoryDelete,
  onProjectMemoryRefresh,
}: SettingsPanelProps) {
  const [activeKey, setActiveKey] = useState(settings.activeKey);
  const [siderWidth, setSiderWidth] = useState(256);
  const menuItems: MenuProps['items'] = settings.navItems.map((item) => ({
    key: item.key,
    icon: item.icon,
    label: item.label,
  }));

  const renderContent = () => {
    switch (activeKey) {
      case 'providers':
        return (
          <ProvidersSettings
            settings={settings}
            onProviderDelete={onProviderDelete}
            onProviderDiscoverDraftModels={onProviderDiscoverDraftModels}
            onProviderDiscoverModels={onProviderDiscoverModels}
            onProviderDraftLatency={onProviderDraftLatency}
            onProviderDraftTest={onProviderDraftTest}
            onProviderLatency={onProviderLatency}
            onProviderSave={onProviderSave}
            onProviderTest={onProviderTest}
            selectedModel={selectedModel}
            onModelSelect={onModelSelect}
            onSettingsRefresh={onSettingsRefresh}
          />
        );
      case 'skills':
        return <SkillsSettings settings={settings} onSkillRefresh={onSkillRefresh} onSkillToggle={onSkillToggle} />;
      case 'mcp':
        return (
          <MCPSettings
            settings={settings}
            onMCPServerDetailsLoad={onMCPServerDetailsLoad}
            onMCPServerRefresh={onMCPServerRefresh}
            onMCPServerSave={onMCPServerSave}
            onMCPServerToggle={onMCPServerToggle}
            onMCPToolToggle={onMCPToolToggle}
          />
        );
      case 'hooks':
        return <HookSettingsPanel hooks={hooks} hookExecutions={hookExecutions} />;
      case 'memory':
        return (
          <MemorySettings
            project={project}
            onCreate={onProjectMemoryCreate}
            onDelete={onProjectMemoryDelete}
            onDetail={onProjectMemoryDetail}
            onEnabledChange={onProjectMemoryEnabledChange}
            onList={onProjectMemoryList}
            onRefresh={onProjectMemoryRefresh}
            onUpdate={onProjectMemoryUpdate}
          />
        );
      case 'common':
        return <CommonSettings settings={settings} onTerminalProfileSelect={onTerminalProfileSelect} onAppearanceSelect={onAppearanceSelect} onOpenTargetSelect={onOpenTargetSelect} onSettingsRefresh={onSettingsRefresh} />;
      case 'context':
        return (
          <ContextGovernanceSettings
            contextUsage={contextUsage}
            selectedModel={selectedModel}
            onLoad={onContextGovernanceLoad}
            onSave={onContextGovernanceSave}
          />
        );
      case 'im':
        return <EmptySettingsPage title="IM 接入" />;
      case 'terminal':
        return <EmptySettingsPage title="终端" />;
      case 'agents':
        return <EmptySettingsPage title="Agents" />;
      case 'plugins':
        return <EmptySettingsPage title="插件" />;
      case 'computer-use':
        return <EmptySettingsPage title="Computer Use" />;
      case 'diagnostics':
        return <EmptySettingsPage title="诊断" />;
      default:
        return <EmptySettingsPage title={settings.navItems.find((item) => item.key === activeKey)?.label ?? '设置'} />;
    }
  };

  const startSiderWidthDrag = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    const pointerId = event.pointerId;
    const startX = event.clientX;
    const startWidth = siderWidth;
    const target = event.currentTarget;

    target.setPointerCapture(pointerId);

    const updateSiderWidth = (moveEvent: PointerEvent) => {
      const nextWidth = Math.min(360, Math.max(220, startWidth + moveEvent.clientX - startX));
      setSiderWidth(nextWidth);
    };

    const stopSiderWidthDrag = () => {
      target.releasePointerCapture(pointerId);
      window.removeEventListener('pointermove', updateSiderWidth);
      window.removeEventListener('pointerup', stopSiderWidthDrag);
      window.removeEventListener('pointercancel', stopSiderWidthDrag);
      nudgeCursorRecompute();
    };

    window.addEventListener('pointermove', updateSiderWidth);
    window.addEventListener('pointerup', stopSiderWidthDrag);
    window.addEventListener('pointercancel', stopSiderWidthDrag);
  };

  return (
    <ConfigProvider
      theme={{
        components: {
          Button: {
            textHoverBg: 'var(--app-surface-hover)',
          },
          Menu: {
            iconSize: 14,
            itemActiveBg: 'var(--app-surface-hover)',
            itemBg: 'transparent',
            itemHeight: 32,
            itemHoverBg: 'var(--app-surface-hover)',
            itemMarginBlock: 2,
            itemMarginInline: 0,
            itemPaddingInline: 8,
            itemSelectedBg: 'var(--app-surface-active)',
            itemSelectedColor: 'var(--app-text-primary)',
          },
        },
      }}
    >
      <Layout className={styles.settings} data-testid="settings-panel">
        <Sider className={styles.sider} theme="light" width={siderWidth}>
          <Button block className={styles.backButton} icon={<ArrowLeftOutlined />} type="text" onClick={() => onModeChange('project')}>
            返回应用
          </Button>
          <Menu
            className={styles.menu}
            inlineIndent={8}
            items={menuItems}
            mode="inline"
            selectedKeys={[activeKey]}
            onClick={({ key }) => {
              setActiveKey(key);
              if (key === 'providers' || key === 'skills' || key === 'mcp') {
                void onSettingsRefresh();
              }
            }}
          />
        </Sider>

        <div
          aria-label="调整设置菜单宽度"
          aria-orientation="vertical"
          aria-valuemax={360}
          aria-valuemin={220}
          aria-valuenow={siderWidth}
          className={styles.splitter}
          role="separator"
          tabIndex={0}
          onPointerDown={startSiderWidthDrag}
          onPointerLeave={nudgeCursorRecompute}
        />

        <Content className={styles.content}>
          <div className={styles.inner}>{renderContent()}</div>
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

function EmptySettingsPage({ title }: { title: string }) {
  return (
    <Title level={2}>{title}</Title>
  );
}

function ProvidersSettings({
  settings,
  onSettingsRefresh,
  onProviderSave,
  onProviderDelete,
  onProviderDiscoverDraftModels,
  onProviderDiscoverModels,
  onProviderDraftTest,
  onProviderTest,
  onProviderDraftLatency,
  onProviderLatency,
  selectedModel,
  onModelSelect,
}: {
  settings: SettingsViewModel;
  onSettingsRefresh: () => Promise<SettingsViewModel>;
  onProviderSave: (provider: ConfiguredProviderViewModel & { token?: string }) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDelete: (providerID: string) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDiscoverDraftModels: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderModelDiscoveryViewModel>;
  onProviderDiscoverModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  onProviderDraftTest: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  onProviderTest: (providerID: string) => Promise<ProviderTestViewModel>;
  onProviderDraftLatency: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  onProviderLatency: (providerID: string) => Promise<ProviderTestViewModel>;
  selectedModel?: RuntimeModelOptionViewModel;
  onModelSelect: (configuredProviderID: string, model: string) => Promise<void>;
}) {
  const [runtimeSettings, setRuntimeSettings] = useState<SettingsViewModel | null>(null);
  const [editingProvider, setEditingProvider] = useState<ConfiguredProviderViewModel | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();
  const activeSettings = runtimeSettings ?? settings;
  const providers = activeSettings.providers;
  const configuredProviders = activeSettings.configuredProviders;
  const selectedProviderID = selectedModel?.configuredProviderId;

  const refreshProviderSettings = async () => {
    const nextSettings = await onSettingsRefresh();
    setRuntimeSettings(nextSettings);
    return nextSettings;
  };

  const openAddProvider = async () => {
    let nextSettings = activeSettings;
    if (providers.length === 0) {
      try {
        nextSettings = await refreshProviderSettings();
      } catch {
        messageApi.error('加载服务商失败');
      }
    }
    setEditingProvider(null);
    setRuntimeSettings(nextSettings);
    setModalOpen(true);
  };

  const openEditProvider = async (provider: ConfiguredProviderViewModel) => {
    let nextSettings = activeSettings;
    if (providers.length === 0) {
      try {
        nextSettings = await refreshProviderSettings();
      } catch {
        messageApi.error('加载服务商失败');
      }
    }
    setRuntimeSettings(nextSettings);
    setEditingProvider(provider);
    setModalOpen(true);
  };

  const saveProvider = async (values: ProviderFormValues) => {
    const provider = normalizeConfiguredProvider(values, providers);
    setSaving(true);
    try {
      const configuredProviders = await onProviderSave(editingProvider ? { ...provider, id: editingProvider.id } : provider);
      setRuntimeSettings({ ...activeSettings, configuredProviders });
      setModalOpen(false);
      messageApi.success(editingProvider ? '服务商已保存' : '服务商已添加');
    } catch (error) {
      messageApi.error(formatProviderSaveError(error));
    } finally {
      setSaving(false);
    }
  };

  const deleteProvider = async (providerID: string) => {
    try {
      const configuredProviders = await onProviderDelete(providerID);
      setRuntimeSettings({ ...activeSettings, configuredProviders });
      messageApi.success('服务商已删除');
    } catch {
      messageApi.error('删除服务商失败');
    }
  };

  const selectDefaultProvider = async (provider: ConfiguredProviderViewModel) => {
    if (!provider.defaultModel) {
      messageApi.warning('请先为服务商选择默认模型');
      return;
    }
    try {
      await onModelSelect(provider.id, provider.defaultModel);
      messageApi.success('默认模型已更新');
    } catch {
      messageApi.error('设置默认模型失败');
    }
  };

  return (
    <>
      {messageContextHolder}
      <Flex align="flex-start" className={styles.providerHeader} justify="space-between">
        <Flex vertical gap={2}>
          <Title className={styles.providerTitle} level={2}>
            服务商
          </Title>
          <Text className={styles.providerSubtitle} type="secondary">
            管理 API 服务商以访问模型。
          </Text>
        </Flex>
      </Flex>

      <section className={styles.section}>
        <Flex align="center" className={styles.providerSectionHeader} justify="space-between">
          <Text className={styles.providerSectionTitle} strong>
            服务商
          </Text>
          <Button className={styles.addProviderButton} icon={<PlusOutlined />} onClick={openAddProvider}>
            添加服务商
          </Button>
        </Flex>

        <Card className={styles.settingsListCard} styles={{ body: { padding: 0 } }}>
          <div className={styles.settingsList}>
            {configuredProviders.map((provider) => {
              const catalogProvider = providers.find((item) => item.id === provider.providerId);
              const providerProtocol = provider.protocol || catalogProvider?.type || 'openai-compat';
              const isSelected = selectedProviderID === provider.id;
              return (
                <div key={provider.id} className={`${styles.settingsListItem} ${isSelected ? styles.selectedProviderItem : ''}`}>
                  <div className={styles.settingsListMeta}>
                    <Flex align="center" gap={8} wrap>
                      <Text strong>{provider.name}</Text>
                      {isSelected && <Tag>默认</Tag>}
                    </Flex>
                    <Text type="secondary">{provider.remark || `${catalogProvider?.name ?? provider.providerId} · ${providerProtocol}`}</Text>
                    <Text type="secondary">模型：{provider.defaultModel || '未选择'}</Text>
                  </div>
                  <Flex className={styles.settingsListActions} align="center" gap={8}>
                    <Button disabled={isSelected || !provider.defaultModel} onClick={() => selectDefaultProvider(provider)}>
                      设为默认
                    </Button>
                    <Button icon={<EditOutlined />} type="text" onClick={() => openEditProvider(provider)} />
                    <Button danger icon={<DeleteOutlined />} type="text" onClick={() => deleteProvider(provider.id)} />
                  </Flex>
                </div>
              );
            })}
          </div>
        </Card>
      </section>

      <ProviderEditorModal
        editingProvider={editingProvider}
        open={modalOpen}
        providers={providers}
        saving={saving}
        onCancel={() => setModalOpen(false)}
        onDiscoverDraftModels={onProviderDiscoverDraftModels}
        onDiscoverModels={onProviderDiscoverModels}
        onDraftLatency={onProviderDraftLatency}
        onDraftTest={onProviderDraftTest}
        onLatency={onProviderLatency}
        onSave={saveProvider}
        onTest={onProviderTest}
      />
    </>
  );
}

function ProviderEditorModal({
  editingProvider,
  open,
  providers,
  saving,
  onCancel,
  onSave,
  onDiscoverDraftModels,
  onDiscoverModels,
  onDraftTest,
  onTest,
  onDraftLatency,
  onLatency,
}: {
  editingProvider: ConfiguredProviderViewModel | null;
  open: boolean;
  providers: ProviderCatalogItemViewModel[];
  saving: boolean;
  onCancel: () => void;
  onSave: (values: ProviderFormValues) => Promise<void>;
  onDiscoverDraftModels: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderModelDiscoveryViewModel>;
  onDiscoverModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  onDraftTest: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  onTest: (providerID: string) => Promise<ProviderTestViewModel>;
  onDraftLatency: (request: ProviderDraftDiscoveryRequestViewModel) => Promise<ProviderTestViewModel>;
  onLatency: (providerID: string) => Promise<ProviderTestViewModel>;
}) {
  const [form] = Form.useForm<ProviderFormValues>();
  const [messageApi, messageContextHolder] = message.useMessage();
  const defaultProviderID = providers[0]?.id ?? '';
  const [selectedProviderID, setSelectedProviderID] = useState(defaultProviderID);
  const [runtimeModels, setRuntimeModels] = useState<ProviderModelViewModel[]>([]);
  const [actionLoading, setActionLoading] = useState<'models' | 'test' | 'latency' | null>(null);
  const [currentStep, setCurrentStep] = useState(0);
  const [modelPickerOpen, setModelPickerOpen] = useState(false);
  const [modelSearch, setModelSearch] = useState('');
  const [pendingModelIDs, setPendingModelIDs] = useState<string[]>([]);
  const watchedModels = Form.useWatch('models', form) ?? [];
  const watchedDefaultContextWindow = Form.useWatch('defaultContextWindow', form);
  const selectedProvider = providers.find((provider) => provider.id === selectedProviderID);
  const modelRows = compactProviderModels(watchedModels);
  const modelOptions = getProviderModelOptions(selectedProvider, providerModelIDs(modelRows.length > 0 ? modelRows : runtimeModels));
  const hasInvalidModelLimits = hasInvalidProviderModelLimits(modelRows, watchedDefaultContextWindow);
  const providerOptions = providers.map((provider) => ({
    label: provider.id === 'custom' ? 'Custom' : provider.name,
    value: provider.id,
  }));

  const setModelRows = (models: ProviderModelViewModel[]) => {
    const compact = compactProviderModels(models);
    form.setFieldValue('models', compact);
    setRuntimeModels(compact);
  };

  const updateModelRow = (index: number, patch: Partial<ProviderModelViewModel>) => {
    const next = [...modelRows];
    next[index] = { ...next[index], ...patch };
    setModelRows(next);
  };

  const addModelRow = () => {
    setModelRows([...modelRows, { id: '' }]);
  };

  const removeModelRow = (index: number) => {
    const removed = modelRows[index]?.id;
    const next = modelRows.filter((_, rowIndex) => rowIndex !== index);
    setModelRows(next);
    if (removed && form.getFieldValue('defaultModel') === removed) form.setFieldValue('defaultModel', next[0]?.id);
  };

  const addSelectedModels = () => {
    const discoveredByID = new Map(runtimeModels.map((model) => [model.id, model]));
    const additions = pendingModelIDs.map((id) => discoveredByID.get(id) ?? { id });
    const next = mergeProviderModelRows(modelRows, additions);
    setModelRows(next);
    if (!form.getFieldValue('defaultModel') && next[0]) form.setFieldValue('defaultModel', next[0].id);
    setPendingModelIDs([]);
    setModelPickerOpen(false);
  };

  const applyPreset = (providerID: string) => {
    const preset = providers.find((provider) => provider.id === providerID);
    setSelectedProviderID(providerID);
    setRuntimeModels([]);
    form.setFieldsValue({
      providerId: providerID,
      name: preset?.name ?? 'Custom',
      remark: '',
      apiEndpoint: preset?.apiEndpoint || '',
      protocol: form.getFieldValue('protocol') || 'openai-compat',
      defaultModel: '',
      models: [],
      defaultContextWindow: 200000,
      proxy: '',
      token: '',
    });
  };

  const draftProviderRequest = async () => {
    const values = await form.validateFields(['protocol', 'apiEndpoint', 'token', 'defaultModel', 'proxy']);
    return {
      protocol: values.protocol,
      apiEndpoint: values.apiEndpoint,
      token: typeof values.token === 'string' ? values.token.trim() : '',
      defaultModel: normalizeDefaultModel(values.defaultModel),
      proxy: values.proxy,
    };
  };

  const shouldUseSavedProvider = (request: ProviderDraftDiscoveryRequestViewModel) => {
    return Boolean(editingProvider?.id && !request.token);
  };

  const refreshModels = async () => {
    setActionLoading('models');
    try {
      const values = await form.validateFields(['protocol', 'apiEndpoint', 'token', 'defaultModel', 'proxy']);
      const token = typeof values.token === 'string' ? values.token.trim() : '';
      const result =
        editingProvider?.id && token === ''
          ? await onDiscoverModels(editingProvider.id)
          : await onDiscoverDraftModels({
              protocol: values.protocol,
              apiEndpoint: values.apiEndpoint,
              token,
              defaultModel: normalizeDefaultModel(values.defaultModel),
              proxy: values.proxy,
            });
      setRuntimeModels(result.models);
      setPendingModelIDs([]);
      setModelPickerOpen(true);
      if (result.error) {
        messageApi.warning(`模型列表未完整刷新：${result.error}`);
      } else {
        messageApi.success(`已刷新 ${result.models.length} 个模型`);
      }
    } catch {
      messageApi.error('刷新模型列表失败');
    } finally {
      setActionLoading(null);
    }
  };

  const testProvider = async () => {
    setActionLoading('test');
    try {
      const request = await draftProviderRequest();
      const result = shouldUseSavedProvider(request) ? await onTest(editingProvider?.id ?? '') : await onDraftTest(request);
      if (result.ok) {
        messageApi.success(`测试通过${formatDuration(result.durationMs)}`);
      } else {
        messageApi.error(result.error || '测试失败');
      }
    } catch {
      messageApi.error('测试失败');
    } finally {
      setActionLoading(null);
    }
  };

  const measureLatency = async () => {
    setActionLoading('latency');
    try {
      const request = await draftProviderRequest();
      const result = shouldUseSavedProvider(request) ? await onLatency(editingProvider?.id ?? '') : await onDraftLatency(request);
      if (result.ok) {
        messageApi.success(`测速 ${result.durationMs ?? 0}ms`);
      } else {
        messageApi.error(result.error || '测速失败');
      }
    } catch {
      messageApi.error('测速失败');
    } finally {
      setActionLoading(null);
    }
  };

  const goToNextStep = async () => {
    if (currentStep === 0) await form.validateFields(['providerId', 'name', 'apiEndpoint', 'protocol', 'token']);
    if (currentStep === 1 && modelRows.length === 0) {
      messageApi.warning('请至少添加一个使用模型');
      return;
    }
    setCurrentStep((step) => Math.min(2, step + 1));
  };

  return (
    <Modal
      className={styles.providerModal}
      destroyOnHidden
      footer={[
        <Button key="cancel" onClick={onCancel}>
          取消
        </Button>,
        currentStep > 0 && <Button key="previous" onClick={() => setCurrentStep((step) => step - 1)}>上一步</Button>,
        currentStep < 2
          ? <Button key="next" type="primary" onClick={() => void goToNextStep()}>下一步</Button>
          : <Button key="submit" disabled={hasInvalidModelLimits || modelRows.length === 0} loading={saving} type="primary" onClick={() => form.submit()}>{editingProvider ? '保存' : '添加'}</Button>,
      ]}
      open={open}
      title={editingProvider ? '编辑服务商' : '添加服务商'}
      width={760}
      onCancel={onCancel}
      afterOpenChange={(visible) => {
        if (!visible) {
          return;
        }
        if (editingProvider) {
          setCurrentStep(0);
          setSelectedProviderID(editingProvider.providerId);
          setRuntimeModels(editingProvider.models ?? []);
          form.setFieldsValue(editingProvider);
          return;
        }
        setCurrentStep(0);
        applyPreset(defaultProviderID);
      }}
    >
      {messageContextHolder}
      <Steps className={styles.providerSteps} current={currentStep} items={[{ title: '连接' }, { title: '选择模型' }, { title: '配置确认' }]} size="small" />
      <Form className={styles.providerForm} form={form} layout="vertical" preserve={false} requiredMark={false} onFinish={onSave}>
        <Form.Item name="providerId" hidden>
          <Input />
        </Form.Item>

        <Card className={styles.formCard} styles={{ body: { padding: 18 } }}>
          <div hidden={currentStep !== 0}>
          <Form.Item label="支持的服务商">
            <Select
              showSearch
              optionFilterProp="label"
              options={providerOptions}
              value={selectedProviderID}
              onChange={applyPreset}
            />
          </Form.Item>

          <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="服务商名称" />
          </Form.Item>

          <Form.Item label="备注" name="remark">
            <Input placeholder="可选备注" />
          </Form.Item>

          <Form.Item label="接口地址" name="apiEndpoint" rules={[{ required: true, message: '请输入接口地址' }]}>
            <Input placeholder="https://api.example.com/v1" />
          </Form.Item>

          <Form.Item label="协议" name="protocol" rules={[{ required: true, message: '请选择协议' }]}>
            <Select options={protocolOptions} />
          </Form.Item>

          <Form.Item label="API 密钥" name="token" rules={[{ required: !editingProvider, message: '请输入 API 密钥' }]}>
            <Input.Password
              iconRender={(visible) => (visible ? <EyeTwoTone /> : <EyeInvisibleOutlined />)}
              placeholder="sk-..."
            />
          </Form.Item>
          <Flex gap={8}>
            <Button icon={<ApiOutlined />} loading={actionLoading === 'test'} onClick={() => void testProvider()}>测试连接</Button>
            <Button icon={<ThunderboltOutlined />} loading={actionLoading === 'latency'} onClick={() => void measureLatency()}>测试延迟</Button>
          </Flex>
          </div>

          <div hidden={currentStep !== 1}>
          <Flex align="flex-end" className={styles.providerModelRow} gap={8}>
            <Form.Item className={styles.providerModelSelect} label="默认模型" name="defaultModel">
              <Select
                showSearch
                optionFilterProp="label"
                options={modelOptions}
                placeholder="选择或输入模型"
              />
            </Form.Item>
            <Flex className={styles.providerActionRow} gap={8}>
              <Button aria-label="刷新模型列表" icon={<ReloadOutlined />} loading={actionLoading === 'models'} onClick={refreshModels} />
              <Button aria-label="测试" icon={<ApiOutlined />} loading={actionLoading === 'test'} onClick={testProvider} />
              <Button aria-label="测速" icon={<ThunderboltOutlined />} loading={actionLoading === 'latency'} onClick={measureLatency} />
            </Flex>
          </Flex>

          <Flex align="center" justify="space-between" className={styles.enabledModelHeader}>
            <Text type="secondary">已添加 {modelRows.length} 个使用模型</Text>
            <Button icon={<PlusOutlined />} onClick={() => setModelPickerOpen(true)}>从候选列表添加</Button>
          </Flex>
          <List
            bordered
            dataSource={modelRows}
            locale={{ emptyText: '获取模型列表后，从候选模型中添加需要使用的模型' }}
            renderItem={(row, index) => (
              <List.Item actions={[<Button key="remove" danger size="small" type="text" onClick={() => removeModelRow(index)}>移除</Button>]}>
                <List.Item.Meta title={row.displayName || row.id} description={`上下文 ${formatTokenWindow(row.contextWindow || row.resolvedContextWindow || 200000)}`} />
              </List.Item>
            )}
          />
          </div>

          <div hidden={currentStep !== 2}>
          <Form.Item label="未知模型默认窗口" name="defaultContextWindow">
            <InputNumber
              controls={false}
              min={16000}
              max={10000000}
              placeholder="留空自动解析"
              status={isInvalidContextWindow(watchedDefaultContextWindow) ? 'error' : undefined}
              style={{ width: '100%' }}
            />
          </Form.Item>

          <Flex align="center" justify="space-between">
            <Text type="secondary">{formatProviderModelSummary(modelRows, watchedDefaultContextWindow)}</Text>
            <Button size="small" onClick={addModelRow}>
              添加模型
            </Button>
          </Flex>

          <div className={styles.modelConfigList}>
            {modelRows.map((row, index) => (
              <Card className={styles.modelConfigCard} key={`${row.id || 'model'}-${index}`} size="small">
                <Flex align="center" justify="space-between" gap={12}>
                  <Input value={row.id} placeholder="模型 ID" onChange={(event) => updateModelRow(index, { id: event.target.value })} />
                  <Button danger type="text" onClick={() => removeModelRow(index)}>移除</Button>
                </Flex>
                <div className={styles.modelConfigGrid}>
                  <label><Text type="secondary">上下文窗口</Text><InputNumber controls={false} min={16000} max={10000000} placeholder="200K" status={isInvalidContextWindow(row.contextWindow) ? 'error' : undefined} value={row.contextWindow} onChange={(value) => updateModelRow(index, { contextWindow: value ?? undefined, source: value ? 'user_override' : undefined })} /></label>
                  <label><Text type="secondary">最大输出</Text><InputNumber controls={false} min={1} placeholder="自动" value={row.maxOutputTokens} onChange={(value) => updateModelRow(index, { maxOutputTokens: value ?? undefined })} /></label>
                </div>
              </Card>
            ))}
          </div>

          <Table<ProviderModelViewModel>
            className={styles.providerModelTable}
            columns={[
              {
                title: '模型 ID',
                dataIndex: 'id',
                render: (_value, row, index) => (
                  <Input
                    value={row.id}
                    onChange={(event) => updateModelRow(index, { id: event.target.value })}
                  />
                ),
              },
              {
                title: '上下文窗口',
                dataIndex: 'contextWindow',
                render: (_value, row, index) => (
                  <Flex align="center" gap={8}>
                    <InputNumber
                      controls={false}
                      min={16000}
                      max={10000000}
                      placeholder={formatTokenWindow(row.resolvedContextWindow)}
                      status={isInvalidContextWindow(row.contextWindow) ? 'error' : undefined}
                      value={row.contextWindow}
                      onChange={(value) => updateModelRow(index, { contextWindow: value ?? undefined, source: value ? 'user_override' : undefined })}
                    />
                    <Tag>{formatModelSource(row.source)}</Tag>
                  </Flex>
                ),
              },
              {
                title: '最大输出',
                dataIndex: 'maxOutputTokens',
                render: (_value, row, index) => (
                  <InputNumber
                    controls={false}
                    min={1}
                    placeholder={formatTokenWindow(row.resolvedMaxOutputTokens)}
                    value={row.maxOutputTokens}
                    onChange={(value) => updateModelRow(index, { maxOutputTokens: value ?? undefined })}
                  />
                ),
              },
              {
                title: '',
                dataIndex: 'actions',
                width: 64,
                render: (_value, _row, index) => (
                  <Button danger size="small" type="text" onClick={() => removeModelRow(index)}>
                    删除
                  </Button>
                ),
              },
            ]}
            dataSource={modelRows}
            locale={{ emptyText: '刷新或添加模型后配置窗口' }}
            pagination={false}
            rowKey={(row, index) => `${row.id || 'model'}-${index}`}
            size="small"
          />

          <Collapse
            ghost
            items={[{
              key: 'advanced',
              label: '高级选项',
              children: (
                <Form.Item label="代理" name="proxy">
                  <Input placeholder="http://127.0.0.1:7890" />
                </Form.Item>
              ),
            }]}
          />
          </div>
        </Card>
      </Form>
      <Modal
        centered
        open={modelPickerOpen}
        title="添加使用模型"
        okText="添加选中模型"
        okButtonProps={{ disabled: pendingModelIDs.length === 0 }}
        onCancel={() => setModelPickerOpen(false)}
        onOk={addSelectedModels}
      >
        <Input allowClear placeholder="搜索模型 ID" value={modelSearch} onChange={(event) => setModelSearch(event.target.value)} />
        <List
          className={styles.modelPickerList}
          dataSource={runtimeModels.filter((model) => !modelSearch.trim() || model.id.toLowerCase().includes(modelSearch.trim().toLowerCase()))}
          locale={{ emptyText: '尚未获取模型，请先点击刷新模型列表' }}
          renderItem={(model) => (
            <List.Item>
              <Checkbox
                checked={pendingModelIDs.includes(model.id)}
                disabled={modelRows.some((row) => row.id === model.id)}
                onChange={(event) => setPendingModelIDs((current) => event.target.checked ? [...new Set([...current, model.id])] : current.filter((id) => id !== model.id))}
              >
                {model.displayName || model.id}
              </Checkbox>
            </List.Item>
          )}
        />
      </Modal>
    </Modal>
  );
}

interface MemorySettingsProps {
  project?: ProjectViewModel;
  onList: (projectID: string) => Promise<{ records: ProjectMemoryRecordViewModel[]; root?: string }>;
  onDetail: (memoryID: string) => Promise<ProjectMemoryRecordViewModel>;
  onCreate: (request: ProjectMemoryCreateViewModel) => Promise<ProjectMemoryRecordViewModel>;
  onUpdate: (memoryID: string, request: ProjectMemoryUpdateViewModel) => Promise<ProjectMemoryRecordViewModel>;
  onEnabledChange: (memoryID: string, enabled: boolean) => Promise<ProjectMemoryRecordViewModel>;
  onDelete: (memoryID: string, reason?: string) => Promise<ProjectMemoryRecordViewModel>;
  onRefresh: (projectID: string) => Promise<unknown>;
}

function MemorySettings({ project, onList, onDetail, onCreate, onUpdate, onEnabledChange, onDelete, onRefresh }: MemorySettingsProps) {
  const [records, setRecords] = useState<ProjectMemoryRecordViewModel[]>([]);
  const [root, setRoot] = useState('');
  const [selectedID, setSelectedID] = useState('');
  const [detail, setDetail] = useState<ProjectMemoryRecordViewModel | undefined>();
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState<string | undefined>();
  const [editing, setEditing] = useState(false);
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm<ProjectMemoryCreateViewModel>();
  const projectID = project?.id || '';

  const loadList = async () => {
    if (!projectID) {
      return;
    }
    setLoading(true);
    try {
      const response = await onList(projectID);
      setRecords(response.records);
      setRoot(response.root || '');
      if (!selectedID && response.records[0]) {
        setSelectedID(response.records[0].id);
      }
    } catch (error) {
      void message.error(error instanceof Error ? error.message : 'Failed to load project memory');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    queueMicrotask(() => {
      if (!cancelled) {
        void loadList();
      }
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectID]);

  useEffect(() => {
    let cancelled = false;
    if (!selectedID) {
      queueMicrotask(() => {
        if (!cancelled) {
          setDetail(undefined);
        }
      });
      return () => {
        cancelled = true;
      };
    }
    void onDetail(selectedID)
      .then((record) => {
        if (!cancelled) {
          setDetail(record);
          form.setFieldsValue({
            projectId: record.projectId,
            relativePath: record.relativePath,
            type: record.type,
            title: record.title,
            description: record.description,
            tags: record.tags,
            content: record.content || '',
          });
        }
      })
      .catch((error) => {
        if (!cancelled) {
          void message.error(error instanceof Error ? error.message : 'Failed to load memory detail');
        }
      });
    return () => {
      cancelled = true;
    };
  }, [selectedID, onDetail, form]);

  const filteredRecords = useMemo(() => {
    const query = search.trim().toLowerCase();
    return records.filter((record) => {
      if (typeFilter && record.type !== typeFilter) {
        return false;
      }
      if (!query) {
        return true;
      }
      return [record.title, record.description, record.relativePath, record.tags.join(' '), record.preview || ''].join(' ').toLowerCase().includes(query);
    });
  }, [records, search, typeFilter]);

  const saveMemory = async () => {
    const values = await form.validateFields();
    try {
      const saved = detail?.id
        ? await onUpdate(detail.id, {
            relativePath: values.relativePath,
            type: values.type,
            title: values.title,
            description: values.description,
            tags: values.tags,
            content: values.content,
          })
        : await onCreate({ ...values, projectId: projectID });
      setSelectedID(saved.id);
      setDetail(saved);
      setEditing(false);
      await loadList();
      void message.success('Memory saved');
    } catch (error) {
      void message.error(error instanceof Error ? error.message : 'Failed to save memory');
    }
  };

  const createMemory = () => {
    setSelectedID('');
    setDetail(undefined);
    setEditing(true);
    form.setFieldsValue({
      projectId: projectID,
      type: 'project',
      title: '',
      description: '',
      tags: [],
      content: '',
    });
  };

  if (!projectID) {
    return (
      <>
        <Title level={2}>Project Memory</Title>
        <Text type="secondary">Open a project to manage project-scoped memory.</Text>
      </>
    );
  }

  return (
    <>
      <Flex align="center" justify="space-between" className={styles.providerHeader}>
        <div>
          <Title className={styles.providerTitle} level={2}>
            Project Memory
          </Title>
          <Text className={styles.providerSubtitle} type="secondary">
            {root || project?.path || projectID}
          </Text>
        </div>
        <Flex gap={8}>
          <Button
            icon={<ReloadOutlined />}
            loading={loading}
            onClick={async () => {
              await onRefresh(projectID);
              await loadList();
            }}
          />
          <Button icon={<PlusOutlined />} type="primary" onClick={createMemory}>
            Create
          </Button>
        </Flex>
      </Flex>

      <section className={styles.section}>
        <Flex gap={12} wrap="wrap">
          <Input.Search className={styles.memorySearch} placeholder="Search memory" value={search} onChange={(event) => setSearch(event.target.value)} />
          <Select
            allowClear
            className={styles.memoryTypeFilter}
            placeholder="Type"
            options={[
              { label: 'User', value: 'user' },
              { label: 'Feedback', value: 'feedback' },
              { label: 'Project', value: 'project' },
              { label: 'Reference', value: 'reference' },
            ]}
            value={typeFilter}
            onChange={setTypeFilter}
          />
        </Flex>

        <div className={styles.memoryLayout}>
          <div className={styles.memoryList}>
            {filteredRecords.map((record) => (
              <button
                key={record.id}
                className={`${styles.memoryListItem} ${selectedID === record.id ? styles.memoryListItemSelected : ''}`}
                type="button"
                onClick={() => {
                  setSelectedID(record.id);
                  setEditing(false);
                }}
              >
                <Flex align="center" gap={8} justify="space-between">
                  <Text strong>{record.title}</Text>
                  <Tag>{record.type}</Tag>
                </Flex>
                <Text type="secondary">{record.description}</Text>
                <Flex gap={4} wrap="wrap">
                  {record.tags.map((tag) => (
                    <Tag key={tag}>{tag}</Tag>
                  ))}
                  {!record.enabled && <Tag color="orange">disabled</Tag>}
                  {record.deletedAt && <Tag color="red">deleted</Tag>}
                </Flex>
              </button>
            ))}
          </div>

          <div className={styles.memoryDetail}>
            <Form className={styles.providerForm} form={form} layout="vertical">
              <Flex align="center" justify="space-between">
                <Text strong>{detail?.id ? 'Memory detail' : 'New memory'}</Text>
                <Flex gap={8}>
                  {detail && (
                    <Switch
                      checked={detail.enabled}
                      disabled={Boolean(detail.deletedAt)}
                      onChange={async (enabled) => {
                        const updated = await onEnabledChange(detail.id, enabled);
                        setDetail(updated);
                        await loadList();
                      }}
                    />
                  )}
                  <Button icon={<EditOutlined />} onClick={() => setEditing(true)} />
                  <Button icon={<SaveOutlined />} type="primary" onClick={saveMemory} />
                  {detail && <Button danger icon={<DeleteOutlined />} onClick={() => void onDelete(detail.id, 'settings_panel').then(loadList)} />}
                </Flex>
              </Flex>
              <Form.Item name="title" rules={[{ required: true }]}>
                <Input disabled={!editing} placeholder="Title" />
              </Form.Item>
              <Form.Item name="type" rules={[{ required: true }]}>
                <Select
                  disabled={!editing}
                  options={[
                    { label: 'User', value: 'user' },
                    { label: 'Feedback', value: 'feedback' },
                    { label: 'Project', value: 'project' },
                    { label: 'Reference', value: 'reference' },
                  ]}
                />
              </Form.Item>
              <Form.Item name="relativePath">
                <Input disabled={!editing} placeholder="project/topic.md" />
              </Form.Item>
              <Form.Item name="description" rules={[{ required: true }]}>
                <Input disabled={!editing} placeholder="Description" />
              </Form.Item>
              <Form.Item name="tags">
                <Select disabled={!editing} mode="tags" placeholder="Tags" />
              </Form.Item>
              <Form.Item name="content" rules={[{ required: true }]}>
                <Input.TextArea className={styles.memoryEditor} disabled={!editing} placeholder="Markdown content" rows={12} />
              </Form.Item>
              {!editing && <Paragraph className={styles.memoryPreview}>{detail?.content || detail?.preview || 'No memory selected.'}</Paragraph>}
            </Form>
          </div>
        </div>
      </section>
    </>
  );
}

function getProviderModelOptions(provider?: ProviderCatalogItemViewModel, runtimeModels: string[] = []) {
  return Array.from(new Set([...runtimeModels, provider?.defaultLargeModel, provider?.defaultSmallModel].filter(Boolean))).map((model) => ({
    label: model,
    value: model,
  }));
}

function providerModelIDs(models: ProviderModelViewModel[]) {
  return compactProviderModels(models).map((model) => model.id);
}

function compactProviderModels(models: ProviderModelViewModel[] = []) {
  const seen = new Set<string>();
  const compact: ProviderModelViewModel[] = [];
  for (const model of models) {
    const id = model.id?.trim();
    if (!id || seen.has(id)) {
      continue;
    }
    seen.add(id);
    compact.push({ ...model, id });
  }
  return compact;
}

function mergeProviderModelRows(existing: ProviderModelViewModel[], discovered: ProviderModelViewModel[]) {
  const existingByID = new Map(compactProviderModels(existing).map((model) => [model.id, model]));
  return compactProviderModels(discovered).map((model) => {
    const current = existingByID.get(model.id);
    return {
      ...model,
      contextWindow: current?.contextWindow,
      maxOutputTokens: current?.maxOutputTokens,
      resolvedContextWindow: model.resolvedContextWindow ?? model.contextWindow,
      resolvedMaxOutputTokens: model.resolvedMaxOutputTokens ?? model.maxOutputTokens,
      source: current?.contextWindow ? 'user_override' : model.source,
    };
  });
}

function isInvalidContextWindow(value?: number | null) {
  return typeof value === 'number' && (value < 16000 || value > 10000000);
}

function hasInvalidProviderModelLimits(models: ProviderModelViewModel[], defaultContextWindow?: number | null) {
  return isInvalidContextWindow(defaultContextWindow) || models.some((model) => isInvalidContextWindow(model.contextWindow));
}

function formatModelSource(source?: string) {
  switch (source) {
    case 'user_override':
      return '用户';
    case 'provider_default':
      return '兜底';
    case 'discovered':
      return '自动获取';
    case 'builtin':
      return '内置';
    default:
      return '默认';
  }
}

function formatTokenWindow(value?: number) {
  if (!value) {
    return undefined;
  }
  if (value >= 1000000) {
    return `${Math.round(value / 100000) / 10}M`;
  }
  if (value >= 1000) {
    return `${Math.round(value / 1000)}k`;
  }
  return `${value}`;
}

function formatProviderModelSummary(models: ProviderModelViewModel[], defaultContextWindow?: number | null) {
  const windows = models
    .map((model) => model.contextWindow || model.resolvedContextWindow)
    .filter((value): value is number => Boolean(value));
  const range = windows.length > 0 ? `${formatTokenWindow(Math.min(...windows))}~${formatTokenWindow(Math.max(...windows))}` : '自动解析';
  return `${models.length} 个模型 · 窗口 ${range} · 兜底 ${formatTokenWindow(defaultContextWindow ?? undefined) ?? '未设置'}`;
}

function formatDuration(durationMs?: number) {
  return typeof durationMs === 'number' ? `，${durationMs}ms` : '';
}

function formatProviderSaveError(error: unknown) {
  const message = error instanceof Error ? error.message : '';
  if (message.includes('configured provider name already exists')) {
    return '已存在相同名称的服务商配置';
  }
  return '保存服务商失败';
}

function normalizeConfiguredProvider(
  values: ProviderFormValues,
  providers: ProviderCatalogItemViewModel[],
): ConfiguredProviderViewModel & { token?: string } {
  const preset = providers.find((provider) => provider.id === values.providerId);
  const defaultModel = normalizeDefaultModel(values.defaultModel);
  const models = compactProviderModels([
    ...(values.models ?? []),
    { id: defaultModel ?? '' },
  ]).map((model) => ({
    // Only the user's explicit values are saved; resolved values and source
    // are recomputed by the backend resolution chain.
    id: model.id,
    displayName: model.displayName,
    contextWindow: model.contextWindow,
    maxOutputTokens: model.maxOutputTokens,
    source: model.contextWindow ? 'user_override' : undefined,
  }));
  return {
    id: values.id || values.providerId,
    providerId: values.providerId,
    name: values.name || preset?.name || values.providerId,
    remark: values.remark,
    apiEndpoint: values.apiEndpoint,
    protocol: values.protocol || 'openai-compat',
    defaultModel,
    models,
    defaultContextWindow: values.defaultContextWindow,
    tokenConfigured: Boolean(values.token || values.tokenConfigured),
    token: values.token,
    proxy: values.proxy,
  };
}

function normalizeDefaultModel(model?: string | string[]) {
  if (Array.isArray(model)) {
    return model[model.length - 1] || '';
  }
  return model;
}

function SkillsSettings({
  settings,
  onSkillRefresh,
  onSkillToggle,
}: {
  settings: SettingsViewModel;
  onSkillRefresh: () => Promise<SettingsViewModel>;
  onSkillToggle: (name: string, enabled: boolean) => Promise<SettingsViewModel>;
}) {
  const [runtimeSettings, setRuntimeSettings] = useState<SettingsViewModel | null>(null);
  const [loading, setLoading] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();
  const activeSettings = runtimeSettings ?? settings;
  const skills = activeSettings.skills;

  const refreshSkills = async () => {
    setLoading(true);
    try {
      const nextSettings = await onSkillRefresh();
      setRuntimeSettings(nextSettings);
      messageApi.success('技能已刷新');
    } catch {
      messageApi.error('刷新技能失败');
    } finally {
      setLoading(false);
    }
  };

  const toggleSkill = async (name: string, enabled: boolean) => {
    try {
      const nextSettings = await onSkillToggle(name, enabled);
      setRuntimeSettings(nextSettings);
      messageApi.success(enabled ? '技能已启用' : '技能已停用');
    } catch {
      messageApi.error('更新技能状态失败');
    }
  };

  return (
    <>
      {messageContextHolder}
      <Flex align="flex-start" className={styles.providerHeader} justify="space-between">
        <Flex vertical gap={2}>
          <Title className={styles.providerTitle} level={2}>
            技能
          </Title>
          <Text className={styles.providerSubtitle} type="secondary">
            从 runtime 读取技能清单、状态和工具边界。
          </Text>
        </Flex>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={refreshSkills}>
          刷新
        </Button>
      </Flex>

      <section className={styles.section}>
        <Card className={styles.settingsListCard} styles={{ body: { padding: 0 } }}>
          <div className={styles.settingsList}>
            {skills.length > 0 ? (
              skills.map((skill) => (
                <div key={skill.name} className={styles.settingsListItem}>
                  <div className={styles.settingsListMeta}>
                    <Text strong>{skill.name}</Text>
                    <Flex vertical gap={6}>
                      <Text type="secondary">{skill.description || skill.reason || skill.diagnostics || '无描述'}</Text>
                      <Flex wrap gap={6}>
                        <Tag color={skill.enabled ? 'green' : 'default'}>{skill.enabled ? 'enabled' : 'disabled'}</Tag>
                        <Tag>{skill.builtin ? 'builtin' : 'local'}</Tag>
                        <Tag color={stateTagColor(skill.state)}>{skill.state || 'unknown'}</Tag>
                        {skill.policyRisk ? <Tag color="orange">{skill.policyRisk}</Tag> : null}
                      </Flex>
                      {skill.path || skill.skillFilePath ? (
                        <Text className={styles.monoText} type="secondary">
                          {skill.skillFilePath || skill.path}
                        </Text>
                      ) : null}
                      {skill.allowedTools.length > 0 ? (
                        <Text type="secondary">Allowed tools: {skill.allowedTools.join(', ')}</Text>
                      ) : null}
                      {skill.error ? <Text type="danger">{skill.error}</Text> : null}
                    </Flex>
                  </div>
                  <Flex className={styles.settingsListActions} align="center" gap={8}>
                  <Switch
                    checked={skill.enabled}
                    onChange={(enabled) => {
                      void toggleSkill(skill.name, enabled);
                    }}
                  />
                  </Flex>
                </div>
              ))
            ) : (
              <Text className={styles.emptyListText} type="secondary">runtime 未发现技能</Text>
            )}
          </div>
        </Card>
      </section>
    </>
  );
}

function MCPSettings({
  settings,
  onMCPServerRefresh,
  onMCPServerSave,
  onMCPServerToggle,
  onMCPToolToggle,
  onMCPServerDetailsLoad,
}: {
  settings: SettingsViewModel;
  onMCPServerRefresh: (name: string) => Promise<SettingsViewModel>;
  onMCPServerSave: (server: RuntimeMCPServerViewModel) => Promise<SettingsViewModel>;
  onMCPServerToggle: (name: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPToolToggle: (server: string, tool: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPServerDetailsLoad: (name: string) => Promise<SettingsViewModel>;
}) {
  const [runtimeSettings, setRuntimeSettings] = useState<SettingsViewModel | null>(null);
  const [selectedServer, setSelectedServer] = useState<string>(settings.mcpServers[0]?.name ?? '');
  const [editingServer, setEditingServer] = useState<RuntimeMCPServerViewModel | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();
  const activeSettings = runtimeSettings ?? settings;
  const servers = activeSettings.mcpServers;
  const selected = servers.find((server) => server.name === selectedServer) ?? servers[0];
  const detailsName = selected?.name ?? '';
  const tools = detailsName ? activeSettings.mcpToolsByServer[detailsName] ?? [] : [];
  const resources = detailsName ? activeSettings.mcpResourcesByServer[detailsName] ?? [] : [];
  const prompts = detailsName ? activeSettings.mcpPromptsByServer[detailsName] ?? [] : [];

  const loadDetails = async (name: string) => {
    setSelectedServer(name);
    try {
      const nextSettings = await onMCPServerDetailsLoad(name);
      setRuntimeSettings(nextSettings);
    } catch {
      messageApi.error('加载 MCP 详情失败');
    }
  };

  const refreshServer = async (name: string) => {
    try {
      const nextSettings = await onMCPServerRefresh(name);
      setRuntimeSettings(nextSettings);
      messageApi.success('MCP server 已刷新');
      void loadDetails(name);
    } catch {
      messageApi.error('刷新 MCP server 失败');
    }
  };

  const toggleServer = async (name: string, enabled: boolean) => {
    try {
      const nextSettings = await onMCPServerToggle(name, enabled);
      setRuntimeSettings(nextSettings);
      messageApi.success(enabled ? 'MCP server 已启用' : 'MCP server 已停用');
    } catch {
      messageApi.error('更新 MCP server 状态失败');
    }
  };

  const toggleTool = async (server: string, tool: string, enabled: boolean) => {
    try {
      const nextSettings = await onMCPToolToggle(server, tool, enabled);
      setRuntimeSettings(nextSettings);
      messageApi.success(enabled ? 'MCP tool 已启用' : 'MCP tool 已停用');
    } catch {
      messageApi.error('更新 MCP tool 状态失败');
    }
  };

  const saveServer = async (values: MCPServerFormValues) => {
    const server = normalizeMCPServer(values, editingServer);
    setSaving(true);
    try {
      const nextSettings = await onMCPServerSave(server);
      setRuntimeSettings(nextSettings);
      setSelectedServer(server.name);
      setModalOpen(false);
      messageApi.success('MCP server 已保存');
    } catch {
      messageApi.error('保存 MCP server 失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      {messageContextHolder}
      <Flex align="flex-start" className={styles.providerHeader} justify="space-between">
        <Flex vertical gap={2}>
          <Title className={styles.providerTitle} level={2}>
            MCP
          </Title>
          <Text className={styles.providerSubtitle} type="secondary">
            管理 runtime MCP servers、工具、资源和 prompts。
          </Text>
        </Flex>
        <Button
          icon={<PlusOutlined />}
          onClick={() => {
            setEditingServer(null);
            setModalOpen(true);
          }}
        >
          添加
        </Button>
      </Flex>

      <section className={styles.section}>
        <div className={styles.mcpLayout}>
          <Card className={styles.settingsListCard} styles={{ body: { padding: 0 } }}>
            <div className={styles.settingsList}>
              {servers.length > 0 ? (
                servers.map((server) => (
                  <div key={server.name} className={`${styles.settingsListItem} ${server.name === detailsName ? styles.selectedListItem : ''}`}>
                    <div className={styles.settingsListMeta}>
                      <Text strong>{server.name}</Text>
                      <Flex vertical gap={6}>
                        <Flex wrap gap={6}>
                          <Tag>{server.type}</Tag>
                          <Tag color={stateTagColor(server.state)}>{server.state || 'unknown'}</Tag>
                          <Tag>{server.counts.tools} tools</Tag>
                          <Tag>{server.counts.resources} resources</Tag>
                          <Tag>{server.counts.prompts} prompts</Tag>
                        </Flex>
                        <Text className={styles.monoText} type="secondary">
                          {server.url || [server.command, ...server.args].filter(Boolean).join(' ')}
                        </Text>
                        {server.error ? <Text type="danger">{server.error}</Text> : server.diagnostics ? <Text type="secondary">{server.diagnostics}</Text> : null}
                      </Flex>
                    </div>
                    <Flex className={styles.settingsListActions} align="center" gap={8}>
                      <Button icon={<ToolOutlined />} type="text" onClick={() => void loadDetails(server.name)} />
                      <Button icon={<ReloadOutlined />} type="text" onClick={() => void refreshServer(server.name)} />
                      <Button
                        icon={<EditOutlined />}
                        type="text"
                        onClick={() => {
                          setEditingServer(server);
                          setModalOpen(true);
                        }}
                      />
                      <Switch checked={server.enabled} onChange={(enabled) => void toggleServer(server.name, enabled)} />
                    </Flex>
                  </div>
                ))
              ) : (
                <Text className={styles.emptyListText} type="secondary">runtime 未配置 MCP server</Text>
              )}
            </div>
          </Card>

          <Card className={styles.mcpDetailsCard} styles={{ body: { padding: 18 } }}>
            {selected ? (
              <Flex vertical gap={20}>
                <Flex align="center" justify="space-between">
                  <Flex vertical gap={2}>
                    <Text strong>{selected.name}</Text>
                    <Text type="secondary">{selected.reason || selected.diagnostics || selected.state}</Text>
                  </Flex>
                  <Button icon={<ReloadOutlined />} onClick={() => void loadDetails(selected.name)} />
                </Flex>
                <CapabilityList title="Tools" items={tools} onToggle={(tool, enabled) => void toggleTool(selected.name, tool, enabled)} />
                <ReadOnlyCapabilityList title="Resources" items={resources.map((resource) => resource.name || resource.uri)} />
                <ReadOnlyCapabilityList title="Prompts" items={prompts.map((prompt) => prompt.name)} />
              </Flex>
            ) : (
              <Text type="secondary">选择或添加 MCP server 后查看 capability 详情。</Text>
            )}
          </Card>
        </div>
      </section>

      <MCPServerEditorModal
        editingServer={editingServer}
        open={modalOpen}
        saving={saving}
        onCancel={() => setModalOpen(false)}
        onSave={saveServer}
      />
    </>
  );
}

function CapabilityList({
  title,
  items,
  onToggle,
}: {
  title: string;
  items: Array<{ name: string; description?: string; enabled: boolean }>;
  onToggle: (name: string, enabled: boolean) => void;
}) {
  return (
    <Flex vertical gap={8}>
      <Text strong>{title}</Text>
      {items.length === 0 ? (
        <Text type="secondary">暂无数据</Text>
      ) : (
        items.map((item) => (
          <Flex key={item.name} align="center" className={styles.compactListItem} gap={12} justify="space-between">
            <Flex vertical>
              <Text>{item.name}</Text>
              {item.description ? <Text type="secondary">{item.description}</Text> : null}
            </Flex>
            <Switch checked={item.enabled} onChange={(enabled) => onToggle(item.name, enabled)} />
          </Flex>
        ))
      )}
    </Flex>
  );
}

function ReadOnlyCapabilityList({ title, items }: { title: string; items: string[] }) {
  return (
    <Flex vertical gap={8}>
      <Text strong>{title}</Text>
      {items.length === 0 ? (
        <Text type="secondary">暂无数据</Text>
      ) : (
        <Flex wrap gap={6}>
          {items.map((item) => (
            <Tag key={item}>{item}</Tag>
          ))}
        </Flex>
      )}
    </Flex>
  );
}

function MCPServerEditorModal({
  editingServer,
  open,
  saving,
  onCancel,
  onSave,
}: {
  editingServer: RuntimeMCPServerViewModel | null;
  open: boolean;
  saving: boolean;
  onCancel: () => void;
  onSave: (values: MCPServerFormValues) => Promise<void>;
}) {
  const [form] = Form.useForm<MCPServerFormValues>();
  return (
    <Modal
      destroyOnHidden
      footer={[
        <Button key="cancel" onClick={onCancel}>
          取消
        </Button>,
        <Button key="submit" loading={saving} type="primary" onClick={() => form.submit()}>
          保存
        </Button>,
      ]}
      open={open}
      title={editingServer ? '编辑 MCP server' : '添加 MCP server'}
      width={720}
      onCancel={onCancel}
      afterOpenChange={(visible) => {
        if (!visible) {
          return;
        }
        form.setFieldsValue(
          editingServer
            ? { ...editingServer, args: editingServer.args.join(' '), enabled: editingServer.enabled }
            : { name: '', type: 'stdio', command: '', args: '', url: '', enabled: true, state: '' },
        );
      }}
    >
      <Form form={form} layout="vertical" preserve={false} requiredMark={false} onFinish={onSave}>
        <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
          <Input disabled={Boolean(editingServer)} />
        </Form.Item>
        <Form.Item label="类型" name="type" rules={[{ required: true, message: '请选择类型' }]}>
          <Select
            options={[
              { label: 'stdio', value: 'stdio' },
              { label: 'http', value: 'http' },
              { label: 'sse', value: 'sse' },
            ]}
          />
        </Form.Item>
        <Form.Item label="URL" name="url">
          <Input placeholder="https://example.com/mcp" />
        </Form.Item>
        <Form.Item label="Command" name="command">
          <Input placeholder="npx" />
        </Form.Item>
        <Form.Item label="Args" name="args">
          <Input placeholder="-y @modelcontextprotocol/server-filesystem ." />
        </Form.Item>
        <Form.Item label="启用" name="enabled" valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>
    </Modal>
  );
}

function normalizeMCPServer(values: MCPServerFormValues, editingServer: RuntimeMCPServerViewModel | null): RuntimeMCPServerViewModel {
  return {
    ...editingServer,
    name: values.name,
    type: values.type || 'stdio',
    url: values.url,
    command: values.command,
    args: splitArgs(values.args),
    disabled: values.enabled === false,
    enabled: values.enabled !== false,
    state: editingServer?.state || 'configured',
    counts: editingServer?.counts ?? { tools: 0, prompts: 0, resources: 0 },
    diagnostics: editingServer?.diagnostics,
    reason: editingServer?.reason,
    error: editingServer?.error,
    enabledTools: editingServer?.enabledTools ?? [],
    disabledTools: editingServer?.disabledTools ?? [],
  };
}

function splitArgs(args?: string) {
  return (args || '')
    .split(' ')
    .map((arg) => arg.trim())
    .filter(Boolean);
}

function stateTagColor(state?: string) {
  switch (state) {
    case 'connected':
    case 'normal':
    case 'ready':
      return 'green';
    case 'failed':
    case 'error':
      return 'red';
    case 'disabled':
      return 'default';
    default:
      return 'blue';
  }
}

function CommonSettings({
  settings,
  onTerminalProfileSelect,
  onAppearanceSelect,
  onSettingsRefresh,
  onOpenTargetSelect,
}: {
  settings: SettingsViewModel;
  onTerminalProfileSelect: (profileID: string) => Promise<SettingsViewModel>;
  onAppearanceSelect: (appearance: AppearanceSettings) => Promise<SettingsViewModel>;
  onSettingsRefresh: () => Promise<SettingsViewModel>;
  onOpenTargetSelect: (targetID: string) => Promise<SettingsViewModel>;
}) {
  const [terminalProfile, setTerminalProfile] = useState(settings.terminalProfile);
  const [savingTerminalProfile, setSavingTerminalProfile] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();
  const selectedTerminalProfile = savingTerminalProfile ? terminalProfile : settings.terminalProfile || terminalProfile;
  const [savingAppearance, setSavingAppearance] = useState(false);
  const [refreshingTerminals, setRefreshingTerminals] = useState(false);
  const [savingOpenTarget, setSavingOpenTarget] = useState(false);

  const saveTerminalProfile = async (profileID: string) => {
    setTerminalProfile(profileID);
    setSavingTerminalProfile(true);
    try {
      const nextSettings = await onTerminalProfileSelect(profileID);
      setTerminalProfile(nextSettings.terminalProfile);
      messageApi.success('终端配置已保存');
    } catch (error) {
      setTerminalProfile(settings.terminalProfile);
      messageApi.error(error instanceof Error ? error.message : '终端配置保存失败');
    } finally {
      setSavingTerminalProfile(false);
    }
  };

  const saveColorMode = async (colorMode: ColorMode) => {
    setSavingAppearance(true);
    try {
      await onAppearanceSelect({ ...settings.appearance, colorMode });
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '主题设置保存失败');
    } finally {
      setSavingAppearance(false);
    }
  };

  const refreshTerminalProfiles = async () => {
    if (refreshingTerminals) return;
    setRefreshingTerminals(true);
    try {
      await onSettingsRefresh();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '终端环境刷新失败');
    } finally {
      setRefreshingTerminals(false);
    }
  };

  const saveOpenTarget = async (targetID: string) => {
    setSavingOpenTarget(true);
    try {
      await onOpenTargetSelect(targetID);
      messageApi.success('默认打开目标已保存');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '默认打开目标保存失败');
    } finally {
      setSavingOpenTarget(false);
    }
  };

  return (
    <>
      {messageContextHolder}
      <section className={`${styles.section} ${styles.commonSection}`}>
        <Card styles={{ body: { padding: 0 } }}>
          <Flex vertical>
            <Flex className={`${styles.listItem} ${styles.appearanceItem}`} vertical gap={16}>
              <Flex vertical>
                <Text>外观</Text>
                <Text type="secondary">主题配色、界面字体等外观设置</Text>
              </Flex>
              <AppearanceModePicker
                disabled={savingAppearance}
                value={settings.appearance.colorMode}
                onChange={(value) => void saveColorMode(value)}
              />
            </Flex>
            <Flex align="center" className={styles.listItem} gap={16} justify="space-between">
              <Flex vertical>
                <Text>默认打开目标</Text>
                <Text type="secondary">默认打开文件和文件夹的位置</Text>
              </Flex>
              <Select
                loading={savingOpenTarget}
                options={settings.editorOptions}
                popupMatchSelectWidth={false}
                style={{ minWidth: 180 }}
                variant="filled"
                value={settings.defaultEditor}
                onChange={(value) => void saveOpenTarget(value)}
              />
            </Flex>
            <Flex align="center" className={styles.listItem} gap={16} justify="space-between">
              <Flex vertical>
                <Text>终端配置</Text>
                <Text type="secondary">运行本地命令时使用的默认终端</Text>
              </Flex>
              <Select
                loading={savingTerminalProfile || refreshingTerminals}
                options={settings.terminalOptions}
                popupMatchSelectWidth={false}
                style={{ minWidth: 180 }}
                value={selectedTerminalProfile}
                variant="filled"
                onChange={(value) => void saveTerminalProfile(value)}
                onOpenChange={(open) => {
                  if (open) void refreshTerminalProfiles();
                }}
              />
            </Flex>
          </Flex>
        </Card>
      </section>
    </>
  );
}

function AppearanceModePicker({ disabled, value, onChange }: { disabled: boolean; value: ColorMode; onChange: (value: ColorMode) => void }) {
  const options: Array<{ label: string; value: ColorMode }> = [
    { label: '系统', value: 'system' },
    { label: '浅色', value: 'light' },
    { label: '深色', value: 'dark' },
  ];
  return (
    <div aria-label="主题" className={styles.appearancePicker} role="radiogroup">
      {options.map((option) => (
        <button
          aria-checked={value === option.value}
          className={`${styles.appearanceOption} ${value === option.value ? styles.appearanceOptionSelected : ''}`}
          disabled={disabled}
          key={option.value}
          role="radio"
          type="button"
          onClick={() => onChange(option.value)}
        >
          <span className={`${styles.themePreview} ${styles[`themePreview${option.value[0].toUpperCase()}${option.value.slice(1)}`]}`}>
            <span className={styles.themePreviewHeader} />
            <span className={styles.themePreviewPanel}>
              <span /><span /><span />
            </span>
          </span>
          <span>{option.label}</span>
        </button>
      ))}
    </div>
  );
}

function formatContextTokens(tokens: number) {
  if (tokens >= 1000) {
    return `${(tokens / 1000).toFixed(tokens >= 100000 ? 0 : 1)}k`;
  }
  return `${tokens}`;
}

const summaryModelOptions = [
  { label: '跟随会话模型', value: 'session' },
  { label: '小模型', value: 'small' },
];

function ContextGovernanceSettings({
  contextUsage,
  selectedModel,
  onLoad,
  onSave,
}: {
  contextUsage?: ContextUsageViewModel;
  selectedModel?: RuntimeModelOptionViewModel;
  onLoad: () => Promise<ContextGovernanceSettingsViewModel>;
  onSave: (settings: ContextGovernanceSettingsViewModel) => Promise<ContextGovernanceSettingsViewModel>;
}) {
  const [settings, setSettings] = useState<ContextGovernanceSettingsViewModel>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [percentInput, setPercentInput] = useState<number | null>(null);
  const [messageApi, messageContextHolder] = message.useMessage();

  useEffect(() => {
    let cancelled = false;
    onLoad()
      .then((loaded) => {
        if (cancelled) return;
        setSettings(loaded);
        setPercentInput(loaded.autoCompactPercent != null ? Math.round(loaded.autoCompactPercent * 100) : null);
      })
      .catch((error) => {
        if (!cancelled) {
          messageApi.error(error instanceof Error ? error.message : '加载上下文设置失败');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const autoCompactEnabled = settings.autoCompactEnabled ?? true;
  const microcompactEnabled = settings.microcompactEnabled ?? true;
  const microcompactKeepRecent = settings.microcompactKeepRecent ?? 5;
  const summaryModel = settings.summaryModel || 'session';

  const persist = async (next: ContextGovernanceSettingsViewModel) => {
    setSaving(true);
    try {
      const saved = await onSave(next);
      setSettings(saved);
      setPercentInput(saved.autoCompactPercent != null ? Math.round(saved.autoCompactPercent * 100) : null);
      messageApi.success('上下文设置已保存');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : '上下文设置保存失败');
    } finally {
      setSaving(false);
    }
  };

  const triggerDescription = contextUsage
    ? `当前模型${selectedModel?.name ? `「${selectedModel.name}」` : ''}窗口 ${formatContextTokens(contextUsage.contextWindow)} tokens，触发点约 ${formatContextTokens(contextUsage.autoCompactAt)} tokens（剩余 ${contextUsage.percentLeft}%）`
    : '上下文接近上限时自动生成摘要并压缩较早的历史消息，让对话可以无限延续下去';

  return (
    <>
      {messageContextHolder}
      <Title level={2}>上下文</Title>
      <section className={styles.section}>
        <Card loading={loading} styles={{ body: { padding: 0 } }}>
          <Flex vertical>
            <Flex align="center" className={styles.listItem} gap={16} justify="space-between">
              <Flex vertical style={{ maxWidth: 480 }}>
                <Text>自动压缩</Text>
                <Text type="secondary">{triggerDescription}</Text>
              </Flex>
              <Switch
                checked={autoCompactEnabled}
                loading={saving}
                onChange={(checked) => void persist({ ...settings, autoCompactEnabled: checked })}
              />
            </Flex>
          </Flex>
        </Card>
      </section>
      <section className={styles.section}>
        <Collapse
          items={[
            {
              key: 'advanced',
              label: '高级设置',
              children: (
                <Flex vertical gap={16}>
                  <Flex align="center" gap={16} justify="space-between">
                    <Flex vertical>
                      <Text>触发百分比覆盖</Text>
                      <Text type="secondary">留空为自动计算；范围 5% ~ 95%，数值越小触发越早</Text>
                    </Flex>
                    <InputNumber
                      max={95}
                      min={5}
                      style={{ minWidth: 120 }}
                      suffix="%"
                      value={percentInput ?? undefined}
                      onBlur={() => void persist({ ...settings, autoCompactPercent: percentInput == null ? undefined : percentInput / 100 })}
                      onChange={(value) => setPercentInput(value == null ? null : Number(value))}
                    />
                  </Flex>
                  <Flex align="center" justify="space-between">
                    <Flex vertical>
                      <Text>Microcompact</Text>
                      <Text type="secondary">单个 step 内自动裁剪较早的工具结果，减少无谓的上下文占用</Text>
                    </Flex>
                    <Switch
                      checked={microcompactEnabled}
                      loading={saving}
                      onChange={(checked) => void persist({ ...settings, microcompactEnabled: checked })}
                    />
                  </Flex>
                  <Flex align="center" justify="space-between">
                    <Flex vertical>
                      <Text>保留最近消息数</Text>
                      <Text type="secondary">microcompact 始终保留的最近消息条数</Text>
                    </Flex>
                    <InputNumber
                      max={50}
                      min={0}
                      style={{ minWidth: 120 }}
                      value={microcompactKeepRecent}
                      onChange={(value) => void persist({ ...settings, microcompactKeepRecent: value ?? 5 })}
                    />
                  </Flex>
                  <Flex align="center" justify="space-between">
                    <Flex vertical>
                      <Text>摘要模型</Text>
                      <Text type="secondary">手动/自动压缩时用于生成摘要的模型</Text>
                    </Flex>
                    <Select
                      options={summaryModelOptions}
                      style={{ minWidth: 160 }}
                      value={summaryModel}
                      onChange={(value) => void persist({ ...settings, summaryModel: value })}
                    />
                  </Flex>
                </Flex>
              ),
            },
          ]}
        />
      </section>
    </>
  );
}
