import { useState } from 'react';
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
  ThunderboltOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  ConfigProvider,
  Flex,
  Form,
  Input,
  Layout,
  Menu,
  message,
  Modal,
  Segmented,
  Select,
  Switch,
  Tag,
  Typography,
} from 'antd';
import type { MenuProps } from 'antd';
import type {
  ConfiguredProviderViewModel,
  ProviderCatalogItemViewModel,
  ProviderModelDiscoveryViewModel,
  ProviderTestViewModel,
  RuntimeMCPServerViewModel,
  SettingsViewModel,
  WorkbenchMode,
} from '../../runtime/workbenchTypes.ts';
import styles from './SettingsPanel.module.css';

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
  onModeChange: (mode: WorkbenchMode) => void;
  onSettingsRefresh: () => Promise<SettingsViewModel>;
  onProviderSave: (provider: ConfiguredProviderViewModel & { token?: string }) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDelete: (providerID: string) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDiscoverModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  onProviderTest: (providerID: string) => Promise<ProviderTestViewModel>;
  onProviderLatency: (providerID: string) => Promise<ProviderTestViewModel>;
  onPermissionModeSelect: (mode: string) => Promise<void>;
  onSkillRefresh: () => Promise<SettingsViewModel>;
  onSkillToggle: (name: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPServerRefresh: (name: string) => Promise<SettingsViewModel>;
  onMCPServerSave: (server: RuntimeMCPServerViewModel) => Promise<SettingsViewModel>;
  onMCPServerToggle: (name: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPToolToggle: (server: string, tool: string, enabled: boolean) => Promise<SettingsViewModel>;
  onMCPServerDetailsLoad: (name: string) => Promise<SettingsViewModel>;
}

export function SettingsPanel({
  settings,
  onModeChange,
  onSettingsRefresh,
  onProviderSave,
  onProviderDelete,
  onProviderDiscoverModels,
  onProviderTest,
  onProviderLatency,
  onPermissionModeSelect,
  onSkillRefresh,
  onSkillToggle,
  onMCPServerRefresh,
  onMCPServerSave,
  onMCPServerToggle,
  onMCPToolToggle,
  onMCPServerDetailsLoad,
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
            onProviderDiscoverModels={onProviderDiscoverModels}
            onProviderLatency={onProviderLatency}
            onProviderSave={onProviderSave}
            onProviderTest={onProviderTest}
            onSettingsRefresh={onSettingsRefresh}
          />
        );
      case 'permissions':
        return <PermissionsSettings settings={settings} onPermissionModeSelect={onPermissionModeSelect} />;
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
      case 'common':
        return <CommonSettings settings={settings} />;
      default:
        return <GeneralSettings />;
    }
  };

  const startSiderResize = (event: ReactPointerEvent<HTMLDivElement>) => {
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

    const stopSiderResize = () => {
      target.releasePointerCapture(pointerId);
      window.removeEventListener('pointermove', updateSiderWidth);
      window.removeEventListener('pointerup', stopSiderResize);
      window.removeEventListener('pointercancel', stopSiderResize);
    };

    window.addEventListener('pointermove', updateSiderWidth);
    window.addEventListener('pointerup', stopSiderResize);
    window.addEventListener('pointercancel', stopSiderResize);
  };

  return (
    <ConfigProvider
      theme={{
        components: {
          Button: {
            textHoverBg: '#f6f6f6',
          },
          Menu: {
            iconSize: 14,
            itemActiveBg: '#f2f2f2',
            itemBg: 'transparent',
            itemHeight: 32,
            itemHoverBg: '#f6f6f6',
            itemMarginBlock: 2,
            itemMarginInline: 0,
            itemPaddingInline: 8,
            itemSelectedBg: '#f2f2f2',
            itemSelectedColor: 'rgba(0, 0, 0, 0.88)',
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
          className={styles.resizer}
          role="separator"
          tabIndex={0}
          onPointerDown={startSiderResize}
        />

        <Content className={styles.content}>
          <div className={styles.inner}>{renderContent()}</div>
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

function GeneralSettings() {
  return (
    <>
      <Title level={2}>常规</Title>

      <section className={styles.section}>
        <Title level={4}>工作模式</Title>
        <Paragraph type="secondary">选择 Agent Builder 显示多少技术细节</Paragraph>
        <Segmented
          block
          className={styles.modeGroup}
          defaultValue="code"
          options={[
            { label: '适用于编程', value: 'code' },
            { label: '适用于日常工作', value: 'work' },
          ]}
          size="large"
        />
      </section>
    </>
  );
}

function ProvidersSettings({
  settings,
  onSettingsRefresh,
  onProviderSave,
  onProviderDelete,
  onProviderDiscoverModels,
  onProviderTest,
  onProviderLatency,
}: {
  settings: SettingsViewModel;
  onSettingsRefresh: () => Promise<SettingsViewModel>;
  onProviderSave: (provider: ConfiguredProviderViewModel & { token?: string }) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDelete: (providerID: string) => Promise<ConfiguredProviderViewModel[]>;
  onProviderDiscoverModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  onProviderTest: (providerID: string) => Promise<ProviderTestViewModel>;
  onProviderLatency: (providerID: string) => Promise<ProviderTestViewModel>;
}) {
  const [runtimeSettings, setRuntimeSettings] = useState<SettingsViewModel | null>(null);
  const [editingProvider, setEditingProvider] = useState<ConfiguredProviderViewModel | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();
  const activeSettings = runtimeSettings ?? settings;
  const providers = activeSettings.providers;
  const configuredProviders = activeSettings.configuredProviders;

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
    } catch {
      messageApi.error('保存服务商失败');
    } finally {
      setSaving(false);
    }
  };

  const deleteProvider = async (providerID: string) => {
    try {
      await onProviderDelete(providerID);
      messageApi.success('服务商已删除');
    } catch {
      messageApi.error('删除服务商失败');
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
              return (
                <div key={provider.id} className={styles.settingsListItem}>
                  <div className={styles.settingsListMeta}>
                    <Text strong>{provider.name}</Text>
                    <Text type="secondary">{provider.remark || `${catalogProvider?.name ?? provider.providerId} · ${providerProtocol}`}</Text>
                  </div>
                  <Flex className={styles.settingsListActions} align="center" gap={8}>
                    <Button icon={<EditOutlined />} type="text" onClick={() => openEditProvider(provider)} />
                    <Button danger icon={<DeleteOutlined />} type="text" onClick={() => deleteProvider(provider.id)} />
                    <Switch defaultChecked />
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
        onDiscoverModels={onProviderDiscoverModels}
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
  onDiscoverModels,
  onTest,
  onLatency,
}: {
  editingProvider: ConfiguredProviderViewModel | null;
  open: boolean;
  providers: ProviderCatalogItemViewModel[];
  saving: boolean;
  onCancel: () => void;
  onSave: (values: ProviderFormValues) => Promise<void>;
  onDiscoverModels: (providerID: string) => Promise<ProviderModelDiscoveryViewModel>;
  onTest: (providerID: string) => Promise<ProviderTestViewModel>;
  onLatency: (providerID: string) => Promise<ProviderTestViewModel>;
}) {
  const [form] = Form.useForm<ProviderFormValues>();
  const [messageApi, messageContextHolder] = message.useMessage();
  const defaultProviderID = providers[0]?.id ?? '';
  const [selectedProviderID, setSelectedProviderID] = useState(defaultProviderID);
  const [runtimeModels, setRuntimeModels] = useState<string[]>([]);
  const [actionLoading, setActionLoading] = useState<'models' | 'test' | 'latency' | null>(null);
  const selectedProvider = providers.find((provider) => provider.id === selectedProviderID);
  const modelOptions = getProviderModelOptions(selectedProvider, runtimeModels);
  const providerOptions = providers.map((provider) => ({
    label: provider.id === 'custom' ? 'Custom' : provider.name,
    value: provider.id,
  }));

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
      defaultModel: preset?.defaultLargeModel || preset?.defaultSmallModel || '',
      proxy: '',
      token: '',
    });
  };

  const requireSavedProvider = () => {
    if (!editingProvider?.id) {
      messageApi.warning('请先保存服务商');
      return '';
    }
    return editingProvider.id;
  };

  const refreshModels = async () => {
    const providerID = requireSavedProvider();
    if (!providerID) {
      return;
    }
    setActionLoading('models');
    try {
      const result = await onDiscoverModels(providerID);
      setRuntimeModels(result.models);
      if (result.models.length > 0 && !form.getFieldValue('defaultModel')) {
        form.setFieldValue('defaultModel', result.models[0]);
      }
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
    const providerID = requireSavedProvider();
    if (!providerID) {
      return;
    }
    setActionLoading('test');
    try {
      const result = await onTest(providerID);
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
    const providerID = requireSavedProvider();
    if (!providerID) {
      return;
    }
    setActionLoading('latency');
    try {
      const result = await onLatency(providerID);
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

  return (
    <Modal
      className={styles.providerModal}
      destroyOnHidden
      footer={[
        <Button key="cancel" onClick={onCancel}>
          取消
        </Button>,
        <Button key="submit" loading={saving} type="primary" onClick={() => form.submit()}>
          {editingProvider ? '保存' : '添加'}
        </Button>,
      ]}
      open={open}
      title={editingProvider ? '编辑服务商' : '添加服务商'}
      width={880}
      onCancel={onCancel}
      afterOpenChange={(visible) => {
        if (!visible) {
          return;
        }
        if (editingProvider) {
          setSelectedProviderID(editingProvider.providerId);
          setRuntimeModels([]);
          form.setFieldsValue({ ...editingProvider, token: '' });
          return;
        }
        applyPreset(defaultProviderID);
      }}
    >
      {messageContextHolder}
      <Form className={styles.providerForm} form={form} layout="vertical" preserve={false} requiredMark={false} onFinish={onSave}>
        <Form.Item name="providerId" hidden>
          <Input />
        </Form.Item>

        <Card className={styles.formCard} styles={{ body: { padding: 18 } }}>
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
              placeholder={editingProvider?.tokenConfigured ? '已保存，留空则不修改' : 'sk-...'}
            />
          </Form.Item>

          <Flex align="flex-end" className={styles.providerModelRow} gap={8}>
            <Form.Item className={styles.providerModelSelect} label="默认模型" name="defaultModel">
              <Select
                showSearch
                mode={selectedProviderID === 'custom' ? 'tags' : undefined}
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

          <Form.Item label="代理" name="proxy">
            <Input placeholder="http://127.0.0.1:7890" />
          </Form.Item>
        </Card>
      </Form>
    </Modal>
  );
}

function getProviderModelOptions(provider?: ProviderCatalogItemViewModel, runtimeModels: string[] = []) {
  return Array.from(new Set([...runtimeModels, provider?.defaultLargeModel, provider?.defaultSmallModel].filter(Boolean))).map((model) => ({
    label: model,
    value: model,
  }));
}

function formatDuration(durationMs?: number) {
  return typeof durationMs === 'number' ? `，${durationMs}ms` : '';
}

function normalizeConfiguredProvider(values: ProviderFormValues, providers: ProviderCatalogItemViewModel[]): ConfiguredProviderViewModel & { token?: string } {
  const preset = providers.find((provider) => provider.id === values.providerId);
  return {
    id: values.id || values.providerId,
    providerId: values.providerId,
    name: values.name || preset?.name || values.providerId,
    remark: values.remark,
    apiEndpoint: values.apiEndpoint,
    protocol: values.protocol || 'openai-compat',
    defaultModel: normalizeDefaultModel(values.defaultModel),
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

function PermissionsSettings({ settings, onPermissionModeSelect }: { settings: SettingsViewModel; onPermissionModeSelect: (mode: string) => Promise<void> }) {
  const selectedMode = settings.permissionMode?.mode ?? settings.permissionOptions[0]?.mode;
  return (
    <>
      <Title level={2}>权限</Title>
      <section className={styles.section}>
        <Title level={4}>权限模式</Title>
        <Segmented
          className={styles.permissionModeGroup}
          options={settings.permissionOptions.map((option) => ({
            disabled: option.disabled,
            label: option.label,
            value: option.mode,
          }))}
          value={selectedMode}
          onChange={(value) => {
            void onPermissionModeSelect(String(value));
          }}
        />
        <Card styles={{ body: { padding: 0 } }}>
          <Flex vertical>
            {settings.permissions.map((item) => (
              <Flex key={item.key} align="center" className={styles.listItem} gap={16} justify="space-between">
                <Flex vertical>
                  <Text strong>{item.title}</Text>
                  <Text type="secondary">{item.description}</Text>
                </Flex>
                <Switch defaultChecked={item.enabled} />
              </Flex>
            ))}
          </Flex>
        </Card>
      </section>
    </>
  );
}

function CommonSettings({ settings }: { settings: SettingsViewModel }) {
  return (
    <>
      <Title level={2}>通用</Title>
      <section className={styles.section}>
        <Card styles={{ body: { padding: 0 } }}>
          <Flex vertical>
            <Flex align="center" className={styles.listItem} gap={16} justify="space-between">
              <Flex vertical>
                <Text>默认打开目标</Text>
                <Text type="secondary">默认打开文件和文件夹的位置</Text>
              </Flex>
              <Select
                defaultValue={settings.defaultEditor}
                options={settings.editorOptions}
                popupMatchSelectWidth={false}
                style={{ minWidth: 180 }}
                variant="filled"
              />
            </Flex>
            <Flex align="center" className={styles.listItem} gap={16} justify="space-between">
              <Flex vertical>
                <Text>终端配置</Text>
                <Text type="secondary">运行本地命令时使用的默认终端</Text>
              </Flex>
              <Select
                defaultValue={settings.terminalProfile}
                options={settings.terminalOptions}
                popupMatchSelectWidth={false}
                style={{ minWidth: 180 }}
                variant="filled"
              />
            </Flex>
          </Flex>
        </Card>
      </section>
    </>
  );
}
