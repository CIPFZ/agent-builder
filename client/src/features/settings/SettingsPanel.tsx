import { useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import { ArrowLeftOutlined } from '@ant-design/icons';
import { Button, Card, ConfigProvider, Flex, Layout, Menu, Radio, Select, Switch, Typography } from 'antd';
import type { MenuProps } from 'antd';
import type { SettingsViewModel, WorkbenchMode } from '../../runtime/workbenchTypes.ts';
import styles from './SettingsPanel.module.css';

const { Content, Sider } = Layout;
const { Paragraph, Text, Title } = Typography;

interface SettingsPanelProps {
  settings: SettingsViewModel;
  onModeChange: (mode: WorkbenchMode) => void;
}

export function SettingsPanel({ settings, onModeChange }: SettingsPanelProps) {
  const [activeKey, setActiveKey] = useState(settings.activeKey);
  const [siderWidth, setSiderWidth] = useState(256);
  const menuItems: MenuProps['items'] = settings.navItems.map((item) => ({
    key: item.key,
    icon: item.icon,
    label: item.label,
  }));

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
            onClick={({ key }) => setActiveKey(key)}
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
          <div className={styles.inner}>
            <Title level={2}>常规</Title>

            <section className={styles.section}>
              <Title level={4}>工作模式</Title>
              <Paragraph type="secondary">选择 Agent Builder 显示多少技术细节</Paragraph>
              <Radio.Group className={styles.modeGroup} defaultValue="code" optionType="button" size="large">
                <Radio.Button className={styles.modeOption} value="code">
                  适用于编程
                </Radio.Button>
                <Radio.Button className={styles.modeOption} value="work">
                  适用于日常工作
                </Radio.Button>
              </Radio.Group>
            </section>

            <section className={styles.section}>
              <Title level={4}>权限</Title>
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

            <section className={styles.section}>
              <Title level={4}>常规</Title>
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
          </div>
        </Content>
      </Layout>
    </ConfigProvider>
  );
}
