import { useMemo, useState } from 'react';
import {
  AppstoreAddOutlined,
  ArrowLeftOutlined,
  CheckOutlined,
  ChromeOutlined,
  CodeOutlined,
  DownOutlined,
  EllipsisOutlined,
  ExperimentOutlined,
  GlobalOutlined,
  PlusOutlined,
  ReloadOutlined,
  RightOutlined,
  SearchOutlined,
  SettingOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Button, Dropdown, Flex, Input, Modal, Segmented, Switch, Typography, message } from 'antd';
import type { MenuProps } from 'antd';
import type { RuntimePluginViewModel, RuntimeSkillViewModel, SettingsViewModel } from '../../runtime/workbenchTypes.ts';
import styles from './PluginCenter.module.css';

const { Text, Title } = Typography;

interface PluginCenterProps {
  settings: SettingsViewModel;
  onSettingsRefresh: () => Promise<SettingsViewModel>;
  onSkillToggle: (name: string, enabled: boolean) => Promise<SettingsViewModel>;
}

type PluginCenterTab = 'plugins' | 'skills';

type PluginIcon = 'browser' | 'chrome' | 'computer' | 'latex' | 'code' | 'cloud' | 'figma' | 'mcp' | 'skills' | 'default';

export function PluginCenter({ settings, onSettingsRefresh, onSkillToggle }: PluginCenterProps) {
  const [activeTab, setActiveTab] = useState<PluginCenterTab>('plugins');
  const [manageMode, setManageMode] = useState(false);
  const [query, setQuery] = useState('');
  const [pluginSourceFilter, setPluginSourceFilter] = useState('全部');
  const [pluginCategoryFilter, setPluginCategoryFilter] = useState('全部');
  const [skillCategoryFilter, setSkillCategoryFilter] = useState('全部');
  const [selectedPlugin, setSelectedPlugin] = useState<RuntimePluginViewModel | null>(null);
  const [selectedSkill, setSelectedSkill] = useState<RuntimeSkillViewModel | null>(null);
  const [runtimeSettings, setRuntimeSettings] = useState<SettingsViewModel | null>(null);
  const [messageApi, messageContextHolder] = message.useMessage();
  const activeSettings = runtimeSettings ?? settings;
  const filteredPlugins = filterPlugins(activeSettings.plugins, query).filter(
    (plugin) => pluginMatchesSource(plugin, pluginSourceFilter) && pluginMatchesCategory(plugin, pluginCategoryFilter),
  );
  const filteredSkills = filterSkills(activeSettings.skills, query).filter((skill) => skillMatchesCategory(skill, skillCategoryFilter));
  const pluginSources = uniqueValues(activeSettings.plugins.map((plugin) => plugin.source));
  const pluginCategories = uniqueValues(activeSettings.plugins.map((plugin) => plugin.category));
  const createMenu: MenuProps = {
    items: [
      { key: 'plugin', icon: <AppstoreAddOutlined />, label: '创建插件' },
      { key: 'skill', icon: <ToolOutlined />, label: '创建技能' },
    ],
  };
  const sourceMenu: MenuProps = {
    items: [
      {
        key: '全部',
        label: <MenuChoice checked={pluginSourceFilter === '全部'} label="全部" />,
      },
      ...pluginSources.map((source) => ({
        key: source,
        label: <MenuChoice checked={pluginSourceFilter === source} label={source} />,
      })),
      {
        type: 'divider' as const,
      },
      { key: 'add-more', icon: <PlusOutlined />, label: '添加更多' },
    ],
    onClick: ({ key }) => {
      if (key !== 'add-more') {
        setPluginSourceFilter(String(key));
      }
    },
  };
  const pluginCategoryMenu: MenuProps = {
    items: [
      { key: 'title', disabled: true, label: '类别' },
      ...['全部', ...pluginCategories].map((category) => ({
        key: category,
        label: <MenuChoice checked={pluginCategoryFilter === category} label={category} />,
      })),
    ],
    onClick: ({ key }) => {
      if (key !== 'title') {
        setPluginCategoryFilter(String(key));
      }
    },
  };
  const skillCategoryMenu: MenuProps = {
    items: [
      { key: 'title', disabled: true, label: '类别' },
      ...['全部', '推荐', '系统', '个人'].map((category) => ({
        key: category,
        label: <MenuChoice checked={skillCategoryFilter === category} label={category} />,
      })),
    ],
    onClick: ({ key }) => {
      if (key !== 'title') {
        setSkillCategoryFilter(String(key));
      }
    },
  };
  const moreMenu: MenuProps = {
    items: [{ key: 'refresh', icon: <ReloadOutlined />, label: '刷新' }],
    onClick: ({ key }) => {
      if (key === 'refresh') {
        void refresh();
      }
    },
  };

  const groupedPlugins = useMemo(() => groupBy(filteredPlugins, (plugin) => plugin.category), [filteredPlugins]);
  const groupedSkills = useMemo(() => groupSkills(filteredSkills), [filteredSkills]);

  const refresh = async () => {
    try {
      setRuntimeSettings(await onSettingsRefresh());
    } catch {
      messageApi.error('刷新失败');
    }
  };

  const toggleSkill = async (skill: RuntimeSkillViewModel, enabled: boolean) => {
    try {
      setRuntimeSettings(await onSkillToggle(skill.name, enabled));
      messageApi.success(enabled ? '技能已启用' : '技能已停用');
    } catch {
      messageApi.error('更新技能状态失败');
    }
  };

  return (
    <section className={styles.center} data-testid="plugin-center">
      {messageContextHolder}
      {selectedPlugin ? (
        <PluginDetailView plugin={selectedPlugin} onBack={() => setSelectedPlugin(null)} />
      ) : manageMode ? (
        <ManagementView
          activeTab={activeTab}
          settings={activeSettings}
          plugins={filteredPlugins}
          query={query}
          skills={filteredSkills}
          onExit={() => {
            setManageMode(false);
            setActiveTab('plugins');
            setQuery('');
          }}
          onPluginSelect={setSelectedPlugin}
          onQueryChange={setQuery}
          onSkillSelect={setSelectedSkill}
          onSkillToggle={toggleSkill}
          onTabChange={(tab) => {
            setActiveTab(tab);
            setQuery('');
            if (tab === 'skills') {
              void refresh();
            }
          }}
        />
      ) : activeTab === 'plugins' ? (
        <>
          <header className={styles.header}>
            <Segmented
              className={styles.topTabs}
              options={[
                { label: '插件', value: 'plugins' },
                { label: '技能', value: 'skills' },
              ]}
              value={activeTab}
              onChange={(value) => {
                setActiveTab(value as PluginCenterTab);
                setManageMode(false);
                setQuery('');
                if (value === 'skills') {
                  void refresh();
                }
              }}
            />
            <Flex align="center" gap={8}>
              <Button icon={<SettingOutlined />} onClick={() => {
                setManageMode(true);
                setQuery('');
              }}>
                管理
              </Button>
              <Dropdown menu={createMenu} placement="bottomRight" trigger={['click']}>
                <Button>
                  创建 <DownOutlined />
                </Button>
              </Dropdown>
              <Dropdown menu={moreMenu} placement="bottomRight" trigger={['click']}>
                <Button icon={<EllipsisOutlined />} type="text" />
              </Dropdown>
            </Flex>
          </header>
          <PluginBrowseView
            categoryFilter={pluginCategoryFilter}
            categoryMenu={pluginCategoryMenu}
            groupedPlugins={groupedPlugins}
            query={query}
            sourceFilter={pluginSourceFilter}
            sourceMenu={sourceMenu}
            onPluginSelect={setSelectedPlugin}
            onQueryChange={setQuery}
          />
        </>
      ) : (
        <>
          <header className={styles.header}>
            <Segmented
              className={styles.topTabs}
              options={[
                { label: '插件', value: 'plugins' },
                { label: '技能', value: 'skills' },
              ]}
              value={activeTab}
              onChange={(value) => {
                setActiveTab(value as PluginCenterTab);
                setManageMode(false);
                setQuery('');
                if (value === 'skills') {
                  void refresh();
                }
              }}
            />
            <Flex align="center" gap={8}>
              <Button icon={<SettingOutlined />} onClick={() => {
                setManageMode(true);
                setQuery('');
              }}>
                管理
              </Button>
              <Dropdown menu={createMenu} placement="bottomRight" trigger={['click']}>
                <Button>
                  创建 <DownOutlined />
                </Button>
              </Dropdown>
              <Dropdown menu={moreMenu} placement="bottomRight" trigger={['click']}>
                <Button icon={<EllipsisOutlined />} type="text" />
              </Dropdown>
            </Flex>
          </header>
          <SkillBrowseView
            categoryFilter={skillCategoryFilter}
            categoryMenu={skillCategoryMenu}
            groupedSkills={groupedSkills}
            query={query}
            onQueryChange={setQuery}
            onSkillSelect={setSelectedSkill}
            onSkillToggle={toggleSkill}
          />
        </>
      )}
      <SkillDetailModal
        skill={selectedSkill}
        onClose={() => setSelectedSkill(null)}
        onToggle={(skill, enabled) => {
          setSelectedSkill({ ...skill, enabled });
          void toggleSkill(skill, enabled);
        }}
      />
    </section>
  );
}

function PluginBrowseView({
  categoryFilter,
  categoryMenu,
  groupedPlugins,
  query,
  sourceFilter,
  sourceMenu,
  onPluginSelect,
  onQueryChange,
}: {
  categoryFilter: string;
  categoryMenu: MenuProps;
  groupedPlugins: Map<string, RuntimePluginViewModel[]>;
  query: string;
  sourceFilter: string;
  sourceMenu: MenuProps;
  onPluginSelect: (plugin: RuntimePluginViewModel) => void;
  onQueryChange: (value: string) => void;
}) {
  return (
    <main className={styles.content}>
      <Title className={styles.heroTitle} level={1}>
        让 Agent Builder 按你的方式工作
      </Title>
      <div className={styles.searchRow}>
        <Input className={styles.searchInput} placeholder="搜索插件" prefix={<SearchOutlined />} value={query} onChange={(event) => onQueryChange(event.target.value)} />
        <Dropdown menu={sourceMenu} placement="bottomLeft" trigger={['click']}>
          <Button>
            {sourceFilter} <DownOutlined />
          </Button>
        </Dropdown>
        <Dropdown menu={categoryMenu} placement="bottomLeft" trigger={['click']}>
          <Button>
            {categoryFilter} <DownOutlined />
          </Button>
        </Dropdown>
      </div>
      <GroupedPluginList groupedPlugins={groupedPlugins} onPluginSelect={onPluginSelect} />
    </main>
  );
}

function SkillBrowseView({
  categoryFilter,
  categoryMenu,
  groupedSkills,
  query,
  onQueryChange,
  onSkillSelect,
  onSkillToggle,
}: {
  categoryFilter: string;
  categoryMenu: MenuProps;
  groupedSkills: Map<string, RuntimeSkillViewModel[]>;
  query: string;
  onQueryChange: (value: string) => void;
  onSkillSelect: (skill: RuntimeSkillViewModel) => void;
  onSkillToggle: (skill: RuntimeSkillViewModel, enabled: boolean) => void;
}) {
  return (
    <main className={styles.content}>
      <div className={styles.searchRowCompact}>
        <Input className={styles.searchInput} placeholder="搜索技能" prefix={<SearchOutlined />} value={query} onChange={(event) => onQueryChange(event.target.value)} />
        <Dropdown menu={categoryMenu} placement="bottomLeft" trigger={['click']}>
          <Button>
            {categoryFilter} <DownOutlined />
          </Button>
        </Dropdown>
      </div>
      <GroupedSkillList groupedSkills={groupedSkills} onSkillSelect={onSkillSelect} onSkillToggle={onSkillToggle} />
    </main>
  );
}

function ManagementView({
  activeTab,
  settings,
  plugins,
  skills,
  query,
  onQueryChange,
  onExit,
  onPluginSelect,
  onSkillSelect,
  onSkillToggle,
  onTabChange,
}: {
  activeTab: PluginCenterTab;
  settings: SettingsViewModel;
  plugins: RuntimePluginViewModel[];
  skills: RuntimeSkillViewModel[];
  query: string;
  onQueryChange: (value: string) => void;
  onExit: () => void;
  onPluginSelect: (plugin: RuntimePluginViewModel) => void;
  onSkillSelect: (skill: RuntimeSkillViewModel) => void;
  onSkillToggle: (skill: RuntimeSkillViewModel, enabled: boolean) => void;
  onTabChange: (tab: PluginCenterTab) => void;
}) {
  return (
    <main className={styles.content}>
      <div className={styles.manageBreadcrumb}>
        <Button className={styles.breadcrumbBack} type="text" onClick={onExit}>
          插件
        </Button>
        <RightOutlined className={styles.breadcrumbChevron} />
        <Text strong>管理</Text>
      </div>
      <div className={styles.manageToolbar}>
        <Flex align="center" gap={24} wrap>
          <Button className={activeTab === 'plugins' ? styles.activePill : styles.plainTab} type="text" onClick={() => onTabChange('plugins')}>
            插件 {plugins.length}
          </Button>
          <Button className={styles.plainTab} type="text">应用 0</Button>
          <Button className={styles.plainTab} type="text">MCP {settings.mcpServers.length}</Button>
          <Button className={activeTab === 'skills' ? styles.activePill : styles.plainTab} type="text" onClick={() => onTabChange('skills')}>
            技能 {skills.length}
          </Button>
          <Button className={styles.plainTab} type="text">市场 0</Button>
        </Flex>
        <Input className={styles.manageSearch} placeholder={activeTab === 'skills' ? '搜索技能' : '搜索插件'} prefix={<SearchOutlined />} value={query} onChange={(event) => onQueryChange(event.target.value)} />
      </div>
      {activeTab === 'plugins' ? (
        <div className={styles.manageList}>
          {plugins.map((plugin) => (
            <PluginManageRow key={plugin.id} plugin={plugin} onSelect={onPluginSelect} />
          ))}
        </div>
      ) : (
        <div className={styles.manageList}>
          {skills.map((skill) => (
            <SkillManageRow key={skill.name} skill={skill} onSelect={onSkillSelect} onToggle={onSkillToggle} />
          ))}
        </div>
      )}
    </main>
  );
}

function GroupedPluginList({ groupedPlugins, onPluginSelect }: { groupedPlugins: Map<string, RuntimePluginViewModel[]>; onPluginSelect: (plugin: RuntimePluginViewModel) => void }) {
  if (groupedPlugins.size === 0) {
    return (
      <div className={styles.emptyState}>
        <Text strong>暂无插件</Text>
        <Text type="secondary">runtime 尚未发现 MCP server 或插件包能力。</Text>
      </div>
    );
  }

  return (
    <div className={styles.groupedList}>
      {Array.from(groupedPlugins.entries()).map(([category, plugins]) => (
        <section key={category} className={styles.pluginGroup}>
          <Text className={styles.groupTitle}>{category}</Text>
          <div className={styles.twoColumnList}>
            {plugins.map((plugin) => (
              <PluginListItem key={plugin.id} plugin={plugin} onSelect={onPluginSelect} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function GroupedSkillList({
  groupedSkills,
  onSkillSelect,
  onSkillToggle,
}: {
  groupedSkills: Map<string, RuntimeSkillViewModel[]>;
  onSkillSelect: (skill: RuntimeSkillViewModel) => void;
  onSkillToggle: (skill: RuntimeSkillViewModel, enabled: boolean) => void;
}) {
  return (
    <div className={styles.groupedList}>
      {Array.from(groupedSkills.entries()).map(([category, skills]) => (
        <section key={category} className={styles.pluginGroup}>
          <Text className={styles.groupTitle}>{category}</Text>
          <div className={styles.twoColumnList}>
            {skills.map((skill) => (
              <SkillListItem key={skill.name} skill={skill} onSelect={onSkillSelect} onToggle={onSkillToggle} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function PluginListItem({ plugin, onSelect }: { plugin: RuntimePluginViewModel; onSelect: (plugin: RuntimePluginViewModel) => void }) {
  return (
    <button className={styles.listItem} type="button" onClick={() => onSelect(plugin)}>
      <IconBadge icon={plugin.icon} />
      <div className={styles.itemText}>
        <Text strong>{plugin.name}</Text>
        <Text type="secondary">{plugin.description || plugin.state}</Text>
      </div>
      <RightOutlined className={styles.checkIcon} />
    </button>
  );
}

function SkillListItem({
  skill,
  onSelect,
  onToggle,
}: {
  skill: RuntimeSkillViewModel;
  onSelect: (skill: RuntimeSkillViewModel) => void;
  onToggle: (skill: RuntimeSkillViewModel, enabled: boolean) => void;
}) {
  return (
    <button className={styles.listItem} type="button" onClick={() => onSelect(skill)}>
      <IconBadge icon={skill.builtin ? 'code' : 'default'} />
      <div className={styles.itemText}>
        <Text strong>{skill.name}</Text>
        <Text type="secondary">{skill.description || skill.path || skill.state}</Text>
      </div>
      <span onClick={(event) => event.stopPropagation()}>
        <Switch checked={skill.enabled} onChange={(enabled) => onToggle(skill, enabled)} />
      </span>
    </button>
  );
}

function PluginManageRow({ plugin, onSelect }: { plugin: RuntimePluginViewModel; onSelect: (plugin: RuntimePluginViewModel) => void }) {
  return (
    <button className={styles.manageRow} type="button" onClick={() => onSelect(plugin)}>
      <IconBadge icon={plugin.icon} />
      <div className={styles.itemText}>
        <Text strong>{plugin.name}</Text>
        <Text type="secondary">{plugin.description || plugin.state}</Text>
      </div>
      <span onClick={(event) => event.stopPropagation()}>
        <Switch checked={plugin.enabled} />
      </span>
    </button>
  );
}

function SkillManageRow({
  skill,
  onSelect,
  onToggle,
}: {
  skill: RuntimeSkillViewModel;
  onSelect: (skill: RuntimeSkillViewModel) => void;
  onToggle: (skill: RuntimeSkillViewModel, enabled: boolean) => void;
}) {
  return (
    <button className={styles.manageRow} type="button" onClick={() => onSelect(skill)}>
      <IconBadge icon={skill.builtin ? 'code' : 'default'} />
      <div className={styles.itemText}>
        <Text strong>{skill.name}</Text>
        <Text type="secondary">{skill.description || skill.path || skill.state}</Text>
      </div>
      <span onClick={(event) => event.stopPropagation()}>
        <Switch checked={skill.enabled} onChange={(enabled) => onToggle(skill, enabled)} />
      </span>
    </button>
  );
}

function PluginDetailView({ plugin, onBack }: { plugin: RuntimePluginViewModel; onBack: () => void }) {
  return (
    <main className={styles.detailContent}>
      <div className={styles.breadcrumb}>
        <Button icon={<ArrowLeftOutlined />} type="text" onClick={onBack}>
          插件
        </Button>
        <RightOutlined />
        <Text strong>{plugin.name}</Text>
      </div>
      <Button className={styles.detailTryButton} icon={<PlusOutlined />}>
        在对话中试用
      </Button>
      <section className={styles.pluginDetailHero}>
        <IconBadge icon={plugin.icon} />
        <Title level={1}>{plugin.name}</Title>
        <Text type="secondary">{plugin.description || plugin.state}</Text>
      </section>
      <section className={styles.detailPreview}>
        {pluginExamplePrompts(plugin).map((prompt) => (
          <div key={prompt} className={styles.promptBubble}>
            <IconBadge icon={plugin.icon} />
            <Text strong>{plugin.name}</Text>
            <Text>{prompt}</Text>
            <Button icon={<RightOutlined />} shape="circle" type="text" />
          </div>
        ))}
      </section>
      <Text className={styles.detailDescription}>
        {plugin.name} 让 Agent Builder 在受控边界内使用相关能力。你可以决定是否启用它，并在对话中按需试用。
      </Text>
      <section className={styles.detailSection}>
        <Title level={3}>能力 <Text type="secondary">{plugin.skills.length + plugin.mcpServers.length}</Text></Title>
        <div className={styles.detailSkillRow}>
          <IconBadge icon={plugin.icon} />
          <div className={styles.itemText}>
            <Text strong>{plugin.name}</Text>
            <Text type="secondary">{plugin.description || plugin.state}</Text>
          </div>
          <Switch checked={plugin.enabled} />
        </div>
      </section>
      <section className={styles.infoGrid}>
        <div>
          <Title level={3}>信息</Title>
          <InfoField label="类别" value={plugin.category} />
          <InfoField label="开发者" value={plugin.source} />
          <InfoField label="功能" value={pluginCapabilitiesLabel(plugin)} />
        </div>
        <div>
          <Title level={3}>链接</Title>
          <a href="https://example.com" rel="noreferrer" target="_blank">网站</a>
          <a href="https://example.com/privacy" rel="noreferrer" target="_blank">隐私政策</a>
          <a href="https://example.com/terms" rel="noreferrer" target="_blank">服务条款</a>
        </div>
      </section>
    </main>
  );
}

function SkillDetailModal({
  skill,
  onClose,
  onToggle,
}: {
  skill: RuntimeSkillViewModel | null;
  onClose: () => void;
  onToggle: (skill: RuntimeSkillViewModel, enabled: boolean) => void;
}) {
  const actionMenu: MenuProps = {
    items: [{ key: 'open', label: '打开' }],
  };

  return (
    <Modal centered className={styles.skillModal} footer={null} open={Boolean(skill)} width={900} onCancel={onClose}>
      {skill ? (
        <div className={styles.skillDialog}>
          <div className={styles.skillDialogHeader}>
            <IconBadge icon={skill.builtin ? 'code' : 'default'} />
            <div className={styles.skillDialogActions}>
              <Switch checked={skill.enabled} onChange={(enabled) => onToggle(skill, enabled)} />
              <Dropdown menu={actionMenu} placement="bottomRight" trigger={['click']}>
                <Button icon={<EllipsisOutlined />} type="text" />
              </Dropdown>
            </div>
          </div>
          <Title level={2}>
            {skill.name} <Text type="secondary">Skill</Text>
          </Title>
          <Text className={styles.skillSubtitle}>{skill.description || skill.reason || 'Create or update a skill'}</Text>
          <div className={styles.skillReadme}>
            <Text>{skill.description || 'This skill is available to the runtime when enabled.'}</Text>
            <Title level={4}>About Skills</Title>
            <Text>
              技能是独立的能力说明和资源集合，用于扩展 Agent Builder 在特定任务、流程或工具上的行为边界。
            </Text>
            <Title level={4}>Runtime Metadata</Title>
            <InfoField label="状态" value={skill.state || 'unknown'} />
            <InfoField label="类型" value={skill.builtin ? '系统' : '个人'} />
            <InfoField label="路径" value={skill.skillFilePath || skill.path || '未提供'} />
            {skill.allowedTools.length > 0 ? <InfoField label="工具" value={skill.allowedTools.join(', ')} /> : null}
            {skill.policyMode ? <InfoField label="策略" value={skill.policyMode} /> : null}
            {skill.error ? <Text type="danger">{skill.error}</Text> : null}
          </div>
          <div className={styles.skillDialogFooter}>
            <Button danger>卸载</Button>
            <Button className={styles.blackButton} icon={<PlusOutlined />}>
              在对话中试用
            </Button>
          </div>
        </div>
      ) : null}
    </Modal>
  );
}

function InfoField({ label, value }: { label: string; value: string }) {
  return (
    <div className={styles.infoField}>
      <Text type="secondary">{label}</Text>
      <Text>{value}</Text>
    </div>
  );
}

function MenuChoice({ checked, label }: { checked: boolean; label: string }) {
  return (
    <span className={styles.menuChoice}>
      <span>{label}</span>
      {checked ? <CheckOutlined /> : null}
    </span>
  );
}

function IconBadge({ icon }: { icon?: string }) {
  const normalizedIcon = normalizePluginIcon(icon);
  const iconNode = {
    browser: <GlobalOutlined />,
    chrome: <ChromeOutlined />,
    computer: <ExperimentOutlined />,
    latex: <CodeOutlined />,
    code: <ToolOutlined />,
    cloud: <AppstoreAddOutlined />,
    figma: <AppstoreAddOutlined />,
    mcp: <AppstoreAddOutlined />,
    skills: <ToolOutlined />,
    default: <ToolOutlined />,
  }[normalizedIcon];
  return <span className={`${styles.iconBadge} ${styles[`icon-${normalizedIcon}`]}`}>{iconNode}</span>;
}

function filterPlugins(plugins: RuntimePluginViewModel[], query: string) {
  const value = query.trim().toLowerCase();
  if (!value) {
    return plugins;
  }
  return plugins.filter((plugin) => `${plugin.name} ${plugin.description ?? ''} ${plugin.category}`.toLowerCase().includes(value));
}

function filterSkills(skills: RuntimeSkillViewModel[], query: string) {
  const value = query.trim().toLowerCase();
  if (!value) {
    return skills;
  }
  return skills.filter((skill) => `${skill.name} ${skill.description ?? ''} ${skill.path ?? ''}`.toLowerCase().includes(value));
}

function pluginMatchesSource(plugin: RuntimePluginViewModel, source: string) {
  return source === '全部' || plugin.source === source;
}

function pluginMatchesCategory(plugin: RuntimePluginViewModel, category: string) {
  return category === '全部' || plugin.category === category;
}

function skillMatchesCategory(skill: RuntimeSkillViewModel, category: string) {
  if (category === '全部') {
    return true;
  }
  if (category === '推荐') {
    return skill.enabled;
  }
  if (category === '系统') {
    return skill.builtin;
  }
  if (category === '个人') {
    return !skill.builtin;
  }
  return true;
}

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.filter(Boolean))).sort((left, right) => left.localeCompare(right));
}

function normalizePluginIcon(icon?: string): PluginIcon {
  if (
    icon === 'browser' ||
    icon === 'chrome' ||
    icon === 'computer' ||
    icon === 'latex' ||
    icon === 'code' ||
    icon === 'cloud' ||
    icon === 'figma' ||
    icon === 'mcp' ||
    icon === 'skills'
  ) {
    return icon;
  }
  return 'default';
}

function pluginCapabilitiesLabel(plugin: RuntimePluginViewModel) {
  const parts = [];
  if (plugin.skills.length > 0) {
    parts.push(`${plugin.skills.length} skills`);
  }
  if (plugin.mcpServers.length > 0) {
    parts.push(`${plugin.mcpServers.length} MCP`);
  }
  if (plugin.toolCount > 0) {
    parts.push(`${plugin.toolCount} tools`);
  }
  if (plugin.resourceCount > 0) {
    parts.push(`${plugin.resourceCount} resources`);
  }
  if (plugin.promptCount > 0) {
    parts.push(`${plugin.promptCount} prompts`);
  }
  return parts.length > 0 ? parts.join(', ') : plugin.kind;
}

function pluginExamplePrompts(plugin: RuntimePluginViewModel) {
  if (plugin.id === 'computer-use') {
    return ['播放一组帮助我进入工作状态的音乐', '构建并运行我已经打开的项目', '打开记事本并整理这些要点'];
  }
  if (plugin.id === 'browser' || plugin.id === 'chrome') {
    return ['打开目标页面并检查当前状态', '查找页面里的关键错误信息', '帮我完成一次本地页面验证'];
  }
  return ['基于当前项目准备一次试用', '整理相关输入并生成结果', '检查输出并给出下一步建议'];
}

function groupBy<T>(items: T[], key: (item: T) => string) {
  const grouped = new Map<string, T[]>();
  for (const item of items) {
    const group = key(item);
    grouped.set(group, [...(grouped.get(group) ?? []), item]);
  }
  return grouped;
}

function groupSkills(skills: RuntimeSkillViewModel[]) {
  const system = skills.filter((skill) => skill.builtin);
  const local = skills.filter((skill) => !skill.builtin);
  const grouped = new Map<string, RuntimeSkillViewModel[]>();
  if (system.length > 0) {
    grouped.set('系统', system);
  }
  if (local.length > 0) {
    grouped.set('个人', local);
  }
  if (grouped.size === 0) {
    grouped.set('技能', []);
  }
  return grouped;
}

