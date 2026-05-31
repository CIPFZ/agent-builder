import { useCallback, useEffect, useState } from 'react';
import { Button, Tooltip } from 'antd';
import {
  BorderOutlined,
  CloseOutlined,
  LeftOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  MinusOutlined,
  RightOutlined,
  SwitcherOutlined,
} from '@ant-design/icons';
import { Window } from '@wailsio/runtime';
import styles from './DesktopChrome.module.css';

const menuItems = ['文件', '编辑', '查看', '窗口', '帮助'];

interface DesktopChromeProps {
  sidebarCollapsed?: boolean;
  onSidebarToggle?: () => void;
}

function runWindowAction(action: () => Promise<void>) {
  void action().catch(() => {
    // Browser preview has no Wails backend; keep controls inert there.
  });
}

export function DesktopChrome({ sidebarCollapsed = false, onSidebarToggle }: DesktopChromeProps) {
  const [isMaximised, setIsMaximised] = useState(false);

  const refreshMaximisedState = useCallback(() => {
    void Window.IsMaximised()
      .then(setIsMaximised)
      .catch(() => {
        setIsMaximised(false);
      });
  }, []);

  useEffect(() => {
    refreshMaximisedState();
  }, [refreshMaximisedState]);

  const handleToggleMaximise = () => {
    void Window.ToggleMaximise()
      .then(refreshMaximisedState)
      .catch(() => {
        setIsMaximised((value) => !value);
      });
  };

  return (
    <header className={styles.chrome} data-testid="desktop-chrome">
      <div className={styles.windowTools}>
        <Tooltip title={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}>
          <Button
            aria-label={sidebarCollapsed ? '展开侧栏' : '折叠侧栏'}
            icon={sidebarCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            type="text"
            onClick={onSidebarToggle}
          />
        </Tooltip>
        <Button aria-label="后退" icon={<LeftOutlined />} type="text" />
        <Button aria-label="前进" icon={<RightOutlined />} type="text" />
      </div>
      <nav className={styles.menu} aria-label="桌面菜单">
        {menuItems.map((item) => (
          <Button key={item} size="small" type="text">
            {item}
          </Button>
        ))}
      </nav>
      <div className={styles.dragRegion} />
      <div className={styles.windowControls} aria-label="窗口控制">
        <Tooltip title="最小化">
          <Button
            aria-label="最小化"
            icon={<MinusOutlined />}
            type="text"
            onClick={() => runWindowAction(Window.Minimise)}
          />
        </Tooltip>
        <Tooltip title={isMaximised ? '还原' : '最大化'}>
          <Button
            aria-label={isMaximised ? '还原' : '最大化'}
            icon={isMaximised ? <SwitcherOutlined /> : <BorderOutlined />}
            type="text"
            onClick={handleToggleMaximise}
          />
        </Tooltip>
        <Tooltip title="关闭">
          <Button
            aria-label="关闭"
            className={styles.closeButton}
            icon={<CloseOutlined />}
            type="text"
            onClick={() => runWindowAction(Window.Close)}
          />
        </Tooltip>
      </div>
    </header>
  );
}
