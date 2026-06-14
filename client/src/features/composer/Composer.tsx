import { Button, Dropdown, message } from 'antd';
import Sender from '@ant-design/x/es/sender';
import { useEffect, useRef, useState } from 'react';
import {
  ArrowUpOutlined,
  BranchesOutlined,
  DownOutlined,
  FolderOpenOutlined,
  PlusOutlined,
  StopOutlined,
} from '@ant-design/icons';
import type { ComposerViewModel, NewConversationDraftViewModel, ProjectViewModel } from '../../runtime/workbenchTypes.ts';
import { PermissionModeControl } from '../permissions/PermissionModeControl.tsx';
import styles from './Composer.module.css';

interface ComposerProps {
  composer: ComposerViewModel;
  project: ProjectViewModel;
  projects: ProjectViewModel[];
  newConversationDraft?: NewConversationDraftViewModel;
  showProjectContext: boolean;
  onNewConversationDraftChange: (target: NewConversationDraftViewModel) => void;
  onModelSelect: (configuredProviderID: string, model: string) => Promise<void>;
  onPermissionModeSelect: (mode: string) => Promise<void>;
  onCancel: () => Promise<void>;
  onSubmit: (prompt: string) => Promise<void>;
}

const menu = {
  items: [
    {
      key: 'current',
      label: '当前选项',
    },
  ],
};

export function Composer({
  composer,
  project,
  projects,
  newConversationDraft,
  showProjectContext,
  onNewConversationDraftChange,
  onModelSelect,
  onPermissionModeSelect,
  onCancel,
  onSubmit,
}: ComposerProps) {
  const [messageApi, messageContextHolder] = message.useMessage();
  const [draft, setDraft] = useState('');
  const composerRef = useRef<HTMLDivElement | null>(null);
  const selectedProviderID = composer.selectedModel?.configuredProviderId;
  const projectOptions = projects.length > 0 ? projects : project.id ? [project] : [];
  const activeDraftTarget =
    newConversationDraft ?? (project.id ? { active: true, scope: 'project' as const, projectId: project.id } : { active: true, scope: 'standalone' as const });
  const selectedDraftProject = activeDraftTarget.scope === 'project'
    ? projectOptions.find((item) => item.id === activeDraftTarget.projectId) ?? project
    : undefined;
  const draftTargetLabel = activeDraftTarget.scope === 'project'
    ? selectedDraftProject?.name || project.name || 'Project'
    : '不使用项目';
  const visibleModelOptions = selectedProviderID
    ? composer.modelOptions.filter((model) => model.configuredProviderId === selectedProviderID)
    : composer.modelOptions.filter((model) => model.configuredProviderId);
  const canSubmit = draft.trim().length > 0;
  const isBusy = Boolean(composer.busy);
  const warnMissingModel = () => {
    void messageApi.warning('请先在 设置 - 服务商 中配置并选择模型后再使用');
  };
  const submitDraft = () => {
    if (isBusy) {
      void onCancel();
      return;
    }
    const prompt = draft.trim();
    if (!prompt) {
      return;
    }
    if (!composer.selectedModel?.configuredProviderId) {
      warnMissingModel();
      return;
    }
    setDraft('');
    void onSubmit(prompt);
  };
  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key !== 'Enter' || event.shiftKey || event.nativeEvent.isComposing) {
      return;
    }
    event.preventDefault();
    submitDraft();
    return false;
  };
  const modelMenu = {
    items:
      visibleModelOptions.length > 0
        ? visibleModelOptions.map((model) => ({
            key: `${model.configuredProviderId ?? ''}:${model.name}`,
            label: model.name,
            disabled: !model.configuredProviderId,
          }))
        : [
            {
              key: 'empty',
              label: '未配置模型',
              disabled: true,
            },
          ],
    onClick: ({ key }: { key: string }) => {
      const option = visibleModelOptions.find((model) => `${model.configuredProviderId ?? ''}:${model.name}` === key);
      if (!option?.configuredProviderId) {
        return;
      }
      void onModelSelect(option.configuredProviderId, option.name);
    },
  };
  const projectTargetMenu = {
    items: [
      ...projectOptions.map((item) => ({
        key: `project:${item.id}`,
        label: item.name,
        icon: <FolderOpenOutlined />,
      })),
      ...(projectOptions.length > 0 ? [{ type: 'divider' as const }] : []),
      {
        key: 'standalone',
        label: '不使用项目',
        icon: <StopOutlined />,
      },
    ],
    onClick: ({ key }: { key: string }) => {
      if (key === 'standalone') {
        onNewConversationDraftChange({ active: true, scope: 'standalone' });
        return;
      }
      const projectID = key.startsWith('project:') ? key.slice('project:'.length) : '';
      if (!projectID) {
        return;
      }
      onNewConversationDraftChange({ active: true, scope: 'project', projectId: projectID });
    },
  };
  const footer = (
    <div className={styles.footer} data-testid="composer-button-row">
      <div className={styles.leftControls}>
        <Button aria-label="添加上下文" icon={<PlusOutlined />} type="text" />
        <PermissionModeControl composer={composer} onSelect={onPermissionModeSelect} />
      </div>

      <div className={styles.rightControls}>
        <Dropdown menu={modelMenu} trigger={['click']}>
          <Button className={styles.modelButton} type="text">
            <span className={styles.truncatedLabel}>{composer.modelLabel}</span>
            <DownOutlined className={styles.chevron} />
          </Button>
        </Dropdown>
        <Button
          aria-label={isBusy ? '停止' : '发送'}
          className={`${styles.sendButton} ${isBusy ? styles.stopButton : ''}`}
          disabled={!isBusy && !canSubmit}
          icon={isBusy ? <span className={styles.stopIcon} /> : <ArrowUpOutlined />}
          shape="circle"
          type="primary"
          onClick={submitDraft}
        />
      </div>
    </div>
  );
  const focusComposerInput = () => {
    const activeElement = document.activeElement;
    if (
      activeElement instanceof HTMLElement &&
      activeElement !== document.body &&
      !composerRef.current?.contains(activeElement) &&
      (activeElement.matches('input, textarea, [contenteditable="true"], button, [role="button"]') || activeElement.closest('.ant-modal, .ant-dropdown'))
    ) {
      return;
    }
    const input = composerRef.current?.querySelector<HTMLTextAreaElement | HTMLDivElement>('textarea, [contenteditable="true"]');
    input?.focus({ preventScroll: true });
  };
  useEffect(() => {
    const frame = window.requestAnimationFrame(focusComposerInput);
    return () => window.cancelAnimationFrame(frame);
  }, [composer.activeTurnId, composer.selectedModel?.id, composer.permissionMode?.mode]);
  useEffect(() => {
    window.addEventListener('focus', focusComposerInput);
    return () => window.removeEventListener('focus', focusComposerInput);
  });

  return (
    <div ref={composerRef} className={styles.composerWrap} data-testid="composer">
      {messageContextHolder}
      <div className={styles.composerShell}>
        <Sender
          autoSize={{ minRows: 1, maxRows: 8 }}
          className={styles.sender}
          footer={footer}
          onKeyDown={handleKeyDown}
          onSubmit={(value) => {
            if (isBusy) {
              void onCancel();
              return;
            }
            const prompt = value.trim();
            if (!prompt) {
              return;
            }
            if (!composer.selectedModel?.configuredProviderId) {
              warnMissingModel();
              return;
            }
            setDraft('');
            void onSubmit(prompt);
          }}
          onChange={setDraft}
          placeholder={composer.placeholder}
          rootClassName={styles.senderRoot}
          suffix={false}
          value={draft}
        />
        {(showProjectContext || newConversationDraft) && (
          <div className={styles.limitBar} data-testid="composer-limit-bar">
            <Dropdown menu={projectTargetMenu} trigger={['click']}>
              <Button className={styles.limitButton} type="text">
                {activeDraftTarget.scope === 'project' ? <FolderOpenOutlined /> : <StopOutlined />}
                <span>{draftTargetLabel}</span>
                <DownOutlined className={styles.chevron} />
              </Button>
            </Dropdown>
            {activeDraftTarget.scope === 'project' && project.isGitRepository && project.branch && (
              <Dropdown menu={menu} trigger={['click']}>
                <Button className={styles.limitButton} type="text">
                  <BranchesOutlined />
                  <span>{project.branch}</span>
                  <DownOutlined className={styles.chevron} />
                </Button>
              </Dropdown>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
