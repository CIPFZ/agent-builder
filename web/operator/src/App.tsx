import {
  Alert,
  App as AntApp,
  Button,
  Descriptions,
  Drawer,
  Dropdown,
  Input,
  Layout,
  MenuProps,
  Modal,
  Select,
  Space,
  Timeline,
  Tooltip,
} from "antd";
import {
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ApiOutlined,
  AppstoreOutlined,
  CheckOutlined,
  CodeOutlined,
  CopyOutlined,
  DeleteOutlined,
  DislikeOutlined,
  DownOutlined,
  LikeOutlined,
  PaperClipOutlined,
  PlusOutlined,
  ProjectOutlined,
  ReloadOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  ShareAltOutlined,
  ToolOutlined,
} from "@ant-design/icons";
import { Actions, Bubble, Conversations, Sender, Welcome } from "@ant-design/x";
import { useEffect, useMemo, useReducer, useState } from "react";

import { MyclawdClient } from "./lib/client";
import { initialOperatorState, SessionSummary, TranscriptMessage } from "./lib/protocol";
import { operatorReducer } from "./lib/store";
import myclawLogo from "./assets/myclaw-logo.png";

function defaultEndpoint() {
  if (window.location.pathname.startsWith("/operator")) {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${window.location.host}/ws`;
  }
  return "ws://127.0.0.1:18080/ws";
}

export function App() {
  const [state, dispatch] = useReducer(operatorReducer, initialOperatorState);
  const [endpoint, setEndpoint] = useState(defaultEndpoint);
  const [prompt, setPrompt] = useState("");
  const [client, setClient] = useState<MyclawdClient | null>(null);
  const [toolDrawer, setToolDrawer] = useState<string | null>(null);
  const [bootstrapWarnings, setBootstrapWarnings] = useState<string[]>([]);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const { message } = AntApp.useApp();

  useEffect(() => {
    return () => {
      client?.disconnect();
    };
  }, [client]);

  useEffect(() => {
    const media = window.matchMedia("(max-width: 1280px)");
    const sync = (matches: boolean) => setSidebarCollapsed(matches);
    sync(media.matches);
    const listener = (event: MediaQueryListEvent) => {
      sync(event.matches);
    };
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, []);

  const transcript = useMemo(() => {
    if (!state.streaming.content) {
      return state.transcript;
    }
    return [
      ...state.transcript,
      {
        id: "streaming",
        role: "assistant",
        content: state.streaming.content,
      },
    ];
  }, [state.streaming.content, state.transcript]);

  const bubbleItems = useMemo(
    () =>
      transcript.map((item) => {
        const isStreaming = item.id === "streaming";
        return {
          key: item.id,
          role: item.role === "user" ? "user" : "assistant",
          placement: item.role === "user" ? ("end" as const) : ("start" as const),
          content: item.content,
          variant: item.role === "user" ? ("filled" as const) : ("borderless" as const),
          shape: item.role === "user" ? ("round" as const) : ("corner" as const),
          ...(isStreaming ? { streaming: true } : {}),
          footer:
            item.role === "user" ? null : (
              <Actions
                className="message-actions"
                items={[
                  { key: "copy", icon: <CopyOutlined /> },
                  { key: "retry", icon: <ReloadOutlined /> },
                  { key: "like", icon: <LikeOutlined /> },
                  { key: "dislike", icon: <DislikeOutlined /> },
                ]}
              />
            ),
        };
      }),
    [transcript],
  );

  const toolRows = useMemo(() => Object.values(state.tools), [state.tools]);
  const approvalRows = useMemo(() => Object.values(state.approvals), [state.approvals]);
  const subagentRows = useMemo(() => Object.values(state.subagents), [state.subagents]);
  const mcpServers = state.mcp.servers;
  const connected = state.connection.status === "connected";

  const activeSession = state.activeSessionKey ?? state.session.session_key ?? "current";
  const activeSessionSummary = state.sessions.find((item) => item.session_key === activeSession);
  const activeTitle = activeSessionSummary?.title ?? (state.session.session_id ? "Your first chat with myclaw" : "New operator session");

  const sessionItems = useMemo(
    () =>
      state.sessions.length > 0
        ? state.sessions.map((session) => ({
            key: session.session_key,
            label: session.title ?? session.last_user_message ?? (session.is_main ? "Main session" : "New chat"),
            group: "今天",
          }))
        : [
            {
              key: activeSession,
              label: activeTitle,
              group: "今天",
            },
          ],
    [activeSession, activeTitle, state.sessions],
  );

  async function bootstrapRuntime(nextClient: MyclawdClient, sessionKey?: string) {
    const sessionPayload = sessionKey ? { session_key: sessionKey } : {};
    const bootstrapResults = await Promise.allSettled([
      nextClient.request("session_status", sessionPayload).then((result) => {
        dispatch({ type: "session/status", payload: (result.payload ?? {}) as never });
      }),
      nextClient.request("session_list").then((result) => {
        const payload = (result.payload ?? {}) as { sessions?: SessionSummary[] };
        dispatch({ type: "sessions/list", payload: payload.sessions ?? [] });
      }),
      nextClient.request("session_messages", sessionPayload).then((result) => {
        const payload = (result.payload ?? {}) as { session_key?: string; messages?: TranscriptMessage[] };
        dispatch({
          type: "session/messages",
          sessionKey: payload.session_key ?? sessionKey ?? "",
          payload: payload.messages ?? [],
        });
      }),
      nextClient.request("mcp_status").then((result) => {
        dispatch({ type: "mcp/status", payload: (result.payload ?? {}) as never });
      }),
      nextClient.request("approval_list").then((result) => {
        dispatch({
          type: "approvals/list",
          payload: (((result.payload ?? {}) as { approvals?: unknown[] }).approvals ?? []) as never,
        });
      }),
      nextClient.request("subagent_list").then((result) => {
        const payload = (result.payload ?? {}) as { tasks?: unknown[]; subagents?: unknown[] };
        dispatch({
          type: "subagents/list",
          payload: (payload.subagents ?? payload.tasks ?? []) as never,
        });
      }),
      nextClient.request("orchestration_status").then((result) => {
        dispatch({
          type: "orchestration/status",
          payload: (((result.payload ?? {}) as { runs?: unknown[] }).runs ?? []) as never,
        });
      }),
    ]);
    const warnings = bootstrapResults
      .filter((result): result is PromiseRejectedResult => result.status === "rejected")
      .map((result) => (result.reason instanceof Error ? result.reason.message : String(result.reason)));
    if (warnings.length > 0) {
      setBootstrapWarnings(warnings);
      message.warning("Connected, but some runtime panels could not be initialized");
    }
  }

  async function connect(sessionKey?: string): Promise<MyclawdClient | null> {
    const requestedSessionKey = sessionKey ?? state.activeSessionKey ?? state.session.session_key;
    dispatch({ type: "connection/connecting", endpoint });
    setBootstrapWarnings([]);
    client?.disconnect();
    const nextClient = new MyclawdClient(endpoint);
    nextClient.subscribe((event) => {
      dispatch({ type: "ws/event", message: event });
    });
    try {
      await nextClient.connect({
        role: "sdk",
        client_identity: "react-operator-ui",
        agent_id: "main",
        ...(requestedSessionKey ? { session_key: requestedSessionKey } : {}),
        supports_permission_control: true,
      });
      setClient(nextClient);
      dispatch({ type: "connection/connected", endpoint });
      message.success("Connected to myclawd");
      await bootstrapRuntime(nextClient, requestedSessionKey);
      return nextClient;
    } catch (error) {
      dispatch({ type: "connection/error", error: error instanceof Error ? error.message : "connect failed" });
      message.error(error instanceof Error ? error.message : "connect failed");
      return null;
    }
  }

  async function createSession() {
    const activeClient = client ?? (await connect());
    if (!activeClient) {
      return;
    }
    const result = await activeClient.request("session_new", { agent_id: state.session.agent_id ?? "main" });
    const payload = (result.payload ?? {}) as unknown as SessionSummary;
    if (!payload.session_id || !payload.session_key) {
      throw new Error("session_new response missing session identity");
    }
    dispatch({
      type: "session/created",
      payload: {
        session_id: payload.session_id,
        session_key: payload.session_key,
        agent_id: payload.agent_id,
        is_main: payload.is_main,
        title: "New chat",
        message_count: 0,
      },
    });
    const list = await activeClient.request("session_list");
    dispatch({
      type: "sessions/list",
      payload: (((list.payload ?? {}) as { sessions?: SessionSummary[] }).sessions ?? []),
    });
    const status = await activeClient.request("session_status", { session_key: payload.session_key });
    dispatch({ type: "session/status", payload: (status.payload ?? {}) as never });
  }

  async function activateSession(sessionKey: string) {
    if (sessionKey === activeSession) {
      return;
    }
    dispatch({ type: "session/activate", sessionKey });
    await connect(sessionKey);
  }

  async function deleteSession(sessionKey: string) {
    if (!client) {
      return;
    }
    const target = state.sessions.find((item) => item.session_key === sessionKey);
    if (target?.is_main) {
      message.warning("Main session cannot be deleted");
      return;
    }
    try {
      const deleted = await client.request("session_delete", { session_key: sessionKey });
      const deletePayload = (deleted.payload ?? {}) as { active_session_key?: string };
      const list = await client.request("session_list");
      const sessions = (((list.payload ?? {}) as { sessions?: SessionSummary[] }).sessions ?? []);
      dispatch({
        type: "sessions/list",
        payload: sessions,
      });
      const fallbackSessionKey =
        deletePayload.active_session_key ??
        sessions.find((item) => item.session_key !== sessionKey)?.session_key;
      if (sessionKey === activeSession && fallbackSessionKey) {
        dispatch({ type: "session/activate", sessionKey: fallbackSessionKey });
        const status = await client.request("session_status", { session_key: fallbackSessionKey });
        dispatch({ type: "session/status", payload: (status.payload ?? {}) as never });
        const messages = await client.request("session_messages", { session_key: fallbackSessionKey });
        const messagesPayload = (messages.payload ?? {}) as { session_key?: string; messages?: TranscriptMessage[] };
        dispatch({
          type: "session/messages",
          sessionKey: messagesPayload.session_key ?? fallbackSessionKey,
          payload: messagesPayload.messages ?? [],
        });
      }
      message.success("Session deleted");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "delete session failed");
    }
  }

  function confirmDeleteSession(sessionKey: string) {
    const target = state.sessions.find((item) => item.session_key === sessionKey);
    Modal.confirm({
      title: "Delete conversation?",
      content: target?.title ?? target?.last_user_message ?? "This conversation will be removed.",
      okText: "Delete",
      okButtonProps: { danger: true },
      onOk: () => deleteSession(sessionKey),
    });
  }

  async function sendPrompt(value = prompt) {
    if (!client || !value.trim()) {
      return;
    }
    await client.request("send_message", { content: value.trim() });
    setPrompt("");
  }

  async function handlePermissionMode(mode: string) {
    if (!client) {
      return;
    }
    const result = await client.request("session_set_permission", { mode, session_key: state.activeSessionKey ?? state.session.session_key });
    dispatch({ type: "session/status", payload: { ...state.session, ...result.payload } as never });
  }

  async function handleModelChange(model: string) {
    if (!client) {
      return;
    }
    const result = await client.request("session_set_model", { model, session_key: state.activeSessionKey ?? state.session.session_key });
    dispatch({ type: "session/status", payload: { ...state.session, ...result.payload } as never });
  }

  async function handleApproval(approvalId: string, approve: boolean) {
    if (!client) {
      return;
    }
    await client.request(approve ? "approval_approve" : "approval_reject", {
      approval_id: approvalId,
    });
    const result = await client.request("approval_list");
    dispatch({
      type: "approvals/list",
      payload: (((result.payload ?? {}) as { approvals?: unknown[] }).approvals ?? []) as never,
    });
  }

  const composerMenuItems: MenuProps["items"] = [
    {
      key: "files",
      icon: <PaperClipOutlined />,
      label: "Add files or photos",
      disabled: true,
    },
    {
      key: "project",
      icon: <ProjectOutlined />,
      label: "Add to project",
      disabled: true,
    },
    {
      type: "divider",
    },
    {
      key: "skills",
      icon: <RobotOutlined />,
      label: "Skills",
      children:
        state.skills.derived.length > 0
          ? state.skills.derived.map((skill) => ({
              key: `skill:${skill}`,
              label: skill,
              onClick: () => setPrompt((current) => `${current}${current ? " " : ""}Use skill ${skill}`),
            }))
          : [{ key: "skills-empty", label: "No skills reported", disabled: true }],
    },
    {
      key: "mcp",
      icon: <AppstoreOutlined />,
      label: "MCP connectors",
      children:
        mcpServers.length > 0
          ? mcpServers.map((server) => ({
              key: `mcp:${server.name}`,
              label: `${server.name} ${server.status ? `(${server.status})` : ""}`,
            }))
          : [{ key: "mcp-empty", label: "No MCP servers reported", disabled: true }],
    },
    {
      key: "tools",
      icon: <ToolOutlined />,
      label: "Tool activity",
      children:
        toolRows.length > 0
          ? toolRows.slice(-8).map((tool) => ({
              key: `tool:${tool.tool_use_id}`,
              label: `${tool.tool_name} · ${tool.status}`,
              onClick: () => setToolDrawer(tool.tool_use_id),
            }))
          : [{ key: "tools-empty", label: "No tool activity", disabled: true }],
    },
    {
      key: "approvals",
      icon: <SafetyCertificateOutlined />,
      label: "Approvals",
      children:
        approvalRows.length > 0
          ? approvalRows.map((approval) => ({
              key: `approval:${approval.approval_id}`,
              label: approval.tool_name ?? approval.approval_id,
              children: [
                {
                  key: `approval:${approval.approval_id}:approve`,
                  label: "Approve",
                  onClick: () => handleApproval(approval.approval_id, true),
                },
                {
                  key: `approval:${approval.approval_id}:reject`,
                  label: "Reject",
                  danger: true,
                  onClick: () => handleApproval(approval.approval_id, false),
                },
              ],
            }))
          : [{ key: "approvals-empty", label: "No pending approvals", disabled: true }],
    },
    {
      type: "divider",
    },
    {
      key: "permission",
      icon: <SafetyCertificateOutlined />,
      label: "Permission mode",
      children: [
        {
          key: "permission:workspace-write",
          label: "workspace-write",
          icon: state.session.permission_mode === "workspace-write" ? <CheckOutlined /> : undefined,
          onClick: () => handlePermissionMode("workspace-write"),
        },
        {
          key: "permission:accept-edits",
          label: "accept-edits",
          icon: state.session.permission_mode === "accept-edits" ? <CheckOutlined /> : undefined,
          onClick: () => handlePermissionMode("accept-edits"),
        },
        {
          key: "permission:bypass",
          label: "bypass-permissions",
          icon: state.session.permission_mode === "bypass-permissions" ? <CheckOutlined /> : undefined,
          onClick: () => handlePermissionMode("bypass-permissions"),
        },
      ],
    },
    {
      key: "runtime",
      icon: <ApiOutlined />,
      label: connected ? "Runtime connected" : "Runtime disconnected",
      disabled: true,
    },
  ];

  const connectionLabel = connected ? "Connected" : state.connection.status === "connecting" ? "Connecting" : "Connect";
  const runtimeMenuItems: MenuProps["items"] = [
    {
      key: "endpoint",
      label: (
        <div className="runtime-menu">
          <div className="runtime-menu-title">myclawd connection</div>
          <div className="runtime-menu-status">
            <span className={`status-dot ${connected ? "online" : ""}`} />
            <span>{connectionLabel}</span>
          </div>
          <div className="runtime-menu-label">WebSocket endpoint</div>
          <Input
            value={endpoint}
            onChange={(event) => setEndpoint(event.target.value)}
            size="middle"
            onClick={(event) => event.stopPropagation()}
          />
          <Button className="runtime-connect-button" type="primary" block onClick={() => connect()}>
            {connected ? "Reconnect" : "Connect"}
          </Button>
        </div>
      ),
    },
  ];
  return (
    <>
      <Layout className={`claude-shell ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}>
        <aside className="sidebar">
          <div className="sidebar-topbar">
            <div className="brand-lockup">
              <img className="brand-mark" src={myclawLogo} alt="myclaw" />
              {!sidebarCollapsed ? <span className="brand-word">myclaw</span> : null}
            </div>
            <div className="sidebar-collapsed-actions">
              <button
                className="sidebar-toggle"
                type="button"
                onClick={() => setSidebarCollapsed((current) => !current)}
                aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
              >
                {sidebarCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              </button>
              <button className="sidebar-quick-new" type="button" onClick={createSession} aria-label="New chat">
                <PlusOutlined />
              </button>
            </div>
          </div>
          <nav className="sidebar-nav">
            <button className="nav-item" type="button" onClick={createSession}>
              <PlusOutlined />
              {!sidebarCollapsed ? <span>开启新对话</span> : null}
            </button>
          </nav>

          <section className="sidebar-section">
            <Conversations
              className={`session-list ${sidebarCollapsed ? "collapsed" : ""}`}
              items={sessionItems}
              activeKey={activeSession}
              groupable
              menu={(conversation) => ({
                items: [
                  {
                    key: "delete",
                    label: "Delete",
                    danger: true,
                    icon: <DeleteOutlined />,
                    disabled: state.sessions.find((item) => item.session_key === conversation.key)?.is_main,
                  },
                ],
                onClick: ({ key, domEvent }) => {
                  domEvent.stopPropagation();
                  if (key === "delete") {
                    confirmDeleteSession(conversation.key);
                  }
                },
              })}
              onActiveChange={activateSession}
            />
          </section>

          <div className="sidebar-footer">
            <Dropdown menu={{ items: runtimeMenuItems }} trigger={["click"]} placement="topLeft">
              <button className="profile-row" type="button">
                <img className="avatar-dot" src={myclawLogo} alt="" aria-hidden="true" />
                {!sidebarCollapsed ? <span>operator</span> : null}
                <span className={`status-dot ${connected ? "online" : ""}`} />
                {!sidebarCollapsed ? <span className="connection-text">{connectionLabel}</span> : null}
              </button>
            </Dropdown>
          </div>
        </aside>

        <main className="workspace">
          {sidebarCollapsed ? (
            <div className="floating-sidebar-controls">
              <img className="floating-brand-mark" src={myclawLogo} alt="myclaw" />
              <div className="floating-action-pill">
                <button
                  className="floating-action-button"
                  type="button"
                  onClick={() => setSidebarCollapsed(false)}
                  aria-label="Expand sidebar"
                >
                  <MenuUnfoldOutlined />
                </button>
                <button className="floating-action-button" type="button" onClick={createSession} aria-label="New chat">
                  <PlusOutlined />
                </button>
              </div>
            </div>
          ) : null}
          <header className="chat-header">
            <div className="chat-header-left">
              <button
                className="sidebar-toggle workspace-toggle"
                type="button"
                onClick={() => setSidebarCollapsed((current) => !current)}
                aria-label={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
              >
                {sidebarCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              </button>
              <button className="chat-title" type="button">
                <span>{activeTitle}</span>
              </button>
            </div>
            <div className="chat-header-actions">
              <Tooltip title="Share">
                <Button type="text" icon={<ShareAltOutlined />} />
              </Tooltip>
            </div>
          </header>

          {state.connection.error ? (
            <Alert className="floating-alert" type="error" showIcon message={state.connection.error} />
          ) : null}
          {bootstrapWarnings.length > 0 ? (
            <Alert
              className="floating-alert"
              type="warning"
              showIcon
              message="Some myclawd methods are unavailable"
              description={bootstrapWarnings.join("; ")}
            />
          ) : null}

          <section className={`workspace-body ${transcript.length > 0 ? "chat-scroll" : "empty-workspace"}`}>
            {transcript.length === 0 ? (
              <Welcome
                className="welcome-block"
                variant="borderless"
                title="你好！很高兴见到你！"
                description="有什么我可以帮你的吗？无论是闲聊、解答问题、处理文件、查找资料，还是进行创作或分析，我都会尽力协助你。"
              />
            ) : (
              <Bubble.List className="message-thread" items={bubbleItems} autoScroll />
            )}
          </section>

          <section className="composer-wrap">
            <div className="composer">
              <Sender
                className="composer-sender"
                value={prompt}
                onChange={setPrompt}
                onSubmit={sendPrompt}
                placeholder="给 myclaw 发送消息"
                autoSize={{ minRows: 2, maxRows: 8 }}
                loading={state.connection.status === "connecting"}
                actions={false}
              />
              <div className="composer-toolbar">
                <div className="composer-left">
                  <Dropdown menu={{ items: composerMenuItems }} trigger={["click"]} placement="topLeft">
                    <button className="composer-icon-button" type="button" aria-label="Open tools">
                      <PlusOutlined />
                    </button>
                  </Dropdown>
                  <Select
                    className="model-select"
                    value={state.session.resolved_main_loop_model ?? "default"}
                    onChange={handleModelChange}
                    popupMatchSelectWidth={false}
                    suffixIcon={<DownOutlined />}
                    options={[
                      { value: "default", label: "Default" },
                      { value: "gpt-5.5", label: "GPT-5.5" },
                      { value: "gpt-5.4", label: "GPT-5.4" },
                      { value: "gpt-5.4-mini", label: "Mini" },
                    ]}
                  />
                </div>

                <div className="composer-right">
                  <Button className="send-button" type="primary" onClick={() => sendPrompt()} disabled={!connected || !prompt.trim()}>
                    ↑
                  </Button>
                </div>
              </div>
            </div>

            <p className="fine-print">内容由 AI 生成，请仔细甄别</p>
          </section>
        </main>
      </Layout>

      <Drawer
        open={Boolean(toolDrawer)}
        width={520}
        title={toolDrawer ? state.tools[toolDrawer]?.tool_name : "Tool details"}
        onClose={() => setToolDrawer(null)}
      >
        {toolDrawer ? (
          <Space direction="vertical" style={{ width: "100%" }}>
            <Descriptions size="small" bordered column={1}>
              <Descriptions.Item label="Status">{state.tools[toolDrawer]?.status}</Descriptions.Item>
              <Descriptions.Item label="Input">{state.tools[toolDrawer]?.tool_input ?? "-"}</Descriptions.Item>
              <Descriptions.Item label="Run ID">{state.tools[toolDrawer]?.run_id ?? "-"}</Descriptions.Item>
            </Descriptions>
            <Timeline
              items={(state.tools[toolDrawer]?.progress ?? []).map((item) => ({
                children: `${item.type ?? "update"}: ${item.message ?? ""}`,
              }))}
            />
            <pre>{JSON.stringify(state.tools[toolDrawer]?.structured_content ?? state.tools[toolDrawer]?.meta ?? {}, null, 2)}</pre>
          </Space>
        ) : null}
      </Drawer>

      <div className="sr-state">
        tools {toolRows.length}, approvals {approvalRows.length}, subagents {subagentRows.length}
      </div>
    </>
  );
}
