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
  Select,
  Space,
  Tag,
  Timeline,
  Tooltip,
  Typography,
} from "antd";
import {
  ApiOutlined,
  AppstoreOutlined,
  CheckOutlined,
  CodeOutlined,
  CopyOutlined,
  DislikeOutlined,
  DownOutlined,
  DownloadOutlined,
  FolderOutlined,
  LikeOutlined,
  PaperClipOutlined,
  PlusOutlined,
  ProjectOutlined,
  PushpinOutlined,
  ReloadOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  ShareAltOutlined,
  ToolOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Conversations } from "@ant-design/x";
import { useEffect, useMemo, useReducer, useState } from "react";

import { MyclawdClient } from "./lib/client";
import { initialOperatorState } from "./lib/protocol";
import { operatorReducer } from "./lib/store";

const DEFAULT_ENDPOINT = "ws://127.0.0.1:18080/ws";

export function App() {
  const [state, dispatch] = useReducer(operatorReducer, initialOperatorState);
  const [endpoint, setEndpoint] = useState(DEFAULT_ENDPOINT);
  const [prompt, setPrompt] = useState("");
  const [activeSession] = useState("current");
  const [client, setClient] = useState<MyclawdClient | null>(null);
  const [toolDrawer, setToolDrawer] = useState<string | null>(null);
  const [bootstrapWarnings, setBootstrapWarnings] = useState<string[]>([]);
  const { message } = AntApp.useApp();

  useEffect(() => {
    return () => {
      client?.disconnect();
    };
  }, [client]);

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

  const toolRows = useMemo(() => Object.values(state.tools), [state.tools]);
  const approvalRows = useMemo(() => Object.values(state.approvals), [state.approvals]);
  const subagentRows = useMemo(() => Object.values(state.subagents), [state.subagents]);
  const mcpServers = state.mcp.servers;
  const connected = state.connection.status === "connected";

  const sessionItems = useMemo(
    () => [
      {
        key: activeSession,
        label: state.session.session_id ? "Your first chat with myclaw" : "New operator session",
      },
    ],
    [activeSession, state.session.session_id],
  );

  async function connect() {
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
        supports_permission_control: true,
      });
      setClient(nextClient);
      dispatch({ type: "connection/connected", endpoint });
      message.success("Connected to myclawd");

      const bootstrapResults = await Promise.allSettled([
        nextClient.request("session_status").then((result) => {
          dispatch({ type: "session/status", payload: (result.payload ?? {}) as never });
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
    } catch (error) {
      dispatch({ type: "connection/error", error: error instanceof Error ? error.message : "connect failed" });
      message.error(error instanceof Error ? error.message : "connect failed");
    }
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
    const result = await client.request("session_set_permission", { mode });
    dispatch({ type: "session/status", payload: { ...state.session, ...result.payload } as never });
  }

  async function handleModelChange(model: string) {
    if (!client) {
      return;
    }
    const result = await client.request("session_set_model", { model });
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

  const runtimeMenuItems: MenuProps["items"] = [
    {
      key: "endpoint",
      label: (
        <div className="runtime-menu">
          <div className="runtime-menu-label">myclawd websocket</div>
          <Input
            value={endpoint}
            onChange={(event) => setEndpoint(event.target.value)}
            size="small"
            onClick={(event) => event.stopPropagation()}
          />
        </div>
      ),
    },
    {
      key: "connect",
      label: connected ? "Reconnect" : "Connect",
      onClick: connect,
    },
  ];

  return (
    <>
      <Layout className="claude-shell">
        <aside className="sidebar">
          <nav className="sidebar-nav">
            <button className="nav-item" type="button">
              <PlusOutlined />
              <span>New chat</span>
            </button>
            <button className="nav-item" type="button">
              <FolderOutlined />
              <span>Projects</span>
            </button>
            <button className="nav-item" type="button">
              <AppstoreOutlined />
              <span>Artifacts</span>
            </button>
            <button className="nav-item" type="button">
              <SettingOutlined />
              <span>Customize</span>
            </button>
          </nav>

          <section className="sidebar-section">
            <div className="section-label">Pinned</div>
            <div className="muted-row">
              <PushpinOutlined />
              <span>Drag to pin</span>
            </div>
          </section>

          <section className="sidebar-section">
            <div className="section-label">Recents</div>
            <Conversations
              className="session-list"
              items={sessionItems}
              activeKey={activeSession}
              onActiveChange={() => undefined}
            />
          </section>

          <div className="sidebar-footer">
            <Dropdown menu={{ items: runtimeMenuItems }} trigger={["click"]} placement="topLeft">
              <button className="profile-row" type="button">
                <UserOutlined />
                <span>myclaw</span>
                <span className={`status-dot ${connected ? "online" : ""}`} />
                <DownloadOutlined className="footer-icon" />
              </button>
            </Dropdown>
          </div>
        </aside>

        <main className="workspace">
          <header className="chat-header">
            <button className="chat-title" type="button">
              <span>Your first chat with myclaw</span>
              <DownOutlined />
            </button>
            <Tooltip title="Share">
              <Button type="text" icon={<ShareAltOutlined />} />
            </Tooltip>
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

          <section className={transcript.length > 0 ? "chat-scroll" : "empty-workspace"}>
            {transcript.length === 0 ? (
              <div className="welcome-block">
                <div className="plan-pill">Operator console</div>
                <h1>
                  <span className="claude-mark">✺</span>
                  Evening, JasperLouisa
                </h1>
              </div>
            ) : (
              <div className="message-thread">
                {transcript.map((item) => (
                  <article key={item.id} className={`message-row ${item.role === "user" ? "message-user" : "message-assistant"}`}>
                    <div className="message-bubble">{item.content}</div>
                    {item.role !== "user" ? (
                      <div className="message-actions">
                        <Tooltip title="Copy">
                          <button type="button">
                            <CopyOutlined />
                          </button>
                        </Tooltip>
                        <Tooltip title="Helpful">
                          <button type="button">
                            <LikeOutlined />
                          </button>
                        </Tooltip>
                        <Tooltip title="Not helpful">
                          <button type="button">
                            <DislikeOutlined />
                          </button>
                        </Tooltip>
                        <Tooltip title="Retry">
                          <button type="button">
                            <ReloadOutlined />
                          </button>
                        </Tooltip>
                      </div>
                    ) : null}
                  </article>
                ))}
              </div>
            )}
          </section>

          <section className="composer-wrap">
            <div className="composer">
              <Input.TextArea
                className="composer-input"
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
                onPressEnter={(event) => {
                  if (!event.shiftKey) {
                    event.preventDefault();
                    sendPrompt();
                  }
                }}
                placeholder="How can I help you today?"
                autoSize={{ minRows: 2, maxRows: 8 }}
              />
              <div className="composer-toolbar">
                <Dropdown menu={{ items: composerMenuItems }} trigger={["click"]} placement="topLeft">
                  <button className="composer-icon-button" type="button" aria-label="Open tools">
                    <PlusOutlined />
                  </button>
                </Dropdown>

                <div className="composer-right">
                  <Select
                    className="model-select"
                    value={state.session.resolved_main_loop_model ?? "default"}
                    onChange={handleModelChange}
                    suffixIcon={<DownOutlined />}
                    options={[
                      { value: "default", label: "Default" },
                      { value: "gpt-5.5", label: "GPT-5.5" },
                      { value: "gpt-5.4", label: "GPT-5.4" },
                      { value: "gpt-5.4-mini", label: "Mini" },
                    ]}
                  />
                  <Tooltip title="Search runtime status">
                    <button className="composer-icon-button subtle" type="button">
                      <SearchOutlined />
                    </button>
                  </Tooltip>
                  <Button className="send-button" type="primary" onClick={() => sendPrompt()} disabled={!connected || !prompt.trim()}>
                    Send
                  </Button>
                </div>
              </div>
            </div>

            <div className="mode-row">
              <button type="button">
                <ToolOutlined />
                Write
              </button>
              <button type="button">
                <RobotOutlined />
                Learn
              </button>
              <button type="button">
                <CodeOutlined />
                Code
              </button>
              <button type="button">
                <SafetyCertificateOutlined />
                {state.session.permission_mode ?? "Permissions"}
              </button>
            </div>
            <p className="fine-print">myclaw can make mistakes. Please double-check generated changes.</p>
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
