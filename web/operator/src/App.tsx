import {
  Alert,
  App as AntApp,
  Button,
  Card,
  Col,
  Descriptions,
  Drawer,
  Input,
  Layout,
  Row,
  Select,
  Space,
  Spin,
  Table,
  Tabs,
  Tag,
  Timeline,
  Typography,
} from "antd";
import {
  ApiOutlined,
  CheckCircleOutlined,
  CloudServerOutlined,
  ExclamationCircleOutlined,
  PlayCircleOutlined,
  RobotOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
} from "@ant-design/icons";
import { Bubble, Conversations, Prompts, Sender } from "@ant-design/x";
import { useEffect, useMemo, useReducer, useState } from "react";

import { MyclawdClient } from "./lib/client";
import { initialOperatorState, TranscriptMessage } from "./lib/protocol";
import { operatorReducer } from "./lib/store";

const DEFAULT_ENDPOINT = "ws://127.0.0.1:18080/ws";

function transcriptItems(messages: TranscriptMessage[], streamingContent: string) {
  const items = messages.map((message) => ({
    key: message.id,
    role: message.role === "user" ? "end" : "start",
    placement: message.role === "user" ? ("end" as const) : ("start" as const),
    content: message.content,
  }));
  if (streamingContent) {
    items.push({
      key: "streaming",
      role: "start",
      placement: "start" as const,
      content: streamingContent,
    });
  }
  return items;
}

export function App() {
  const [state, dispatch] = useReducer(operatorReducer, initialOperatorState);
  const [endpoint, setEndpoint] = useState(DEFAULT_ENDPOINT);
  const [prompt, setPrompt] = useState("");
  const [activeSession] = useState("current");
  const [client, setClient] = useState<MyclawdClient | null>(null);
  const [toolDrawer, setToolDrawer] = useState<string | null>(null);
  const [approvalFeedback, setApprovalFeedback] = useState<Record<string, string>>({});
  const [bootstrapWarnings, setBootstrapWarnings] = useState<string[]>([]);
  const { message } = AntApp.useApp();

  useEffect(() => {
    return () => {
      client?.disconnect();
    };
  }, [client]);

  const sessionItems = useMemo(
    () => [
      {
        key: activeSession,
        label: state.session.session_id ? `Session ${state.session.session_id.slice(0, 8)}` : "Current Session",
      },
    ],
    [activeSession, state.session.session_id],
  );

  const toolRows = useMemo(
    () =>
      Object.values(state.tools).sort((a, b) => {
        const aAt = a.progress[a.progress.length - 1]?.at ?? a.result_message?.created_at ?? "";
        const bAt = b.progress[b.progress.length - 1]?.at ?? b.result_message?.created_at ?? "";
        return bAt.localeCompare(aAt);
      }),
    [state.tools],
  );

  const approvalRows = useMemo(() => Object.values(state.approvals), [state.approvals]);
  const subagentRows = useMemo(() => Object.values(state.subagents), [state.subagents]);

  const fileOps = useMemo(
    () =>
      toolRows.filter((tool) =>
        ["Read", "Write", "Edit", "MultiEdit", "Glob", "Grep", "LS"].includes(tool.tool_name),
      ),
    [toolRows],
  );
  const execOps = useMemo(
    () =>
      toolRows.filter((tool) =>
        ["Bash", "Shell", "System", "SSH"].some((name) => tool.tool_name.toLowerCase().includes(name.toLowerCase())),
      ),
    [toolRows],
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

  async function sendPrompt(value: string) {
    if (!client || !value.trim()) {
      return;
    }
    await client.request("send_message", { content: value.trim() });
    setPrompt("");
  }

  async function refreshApprovals() {
    if (!client) {
      return;
    }
    const result = await client.request("approval_list");
    dispatch({
      type: "approvals/list",
      payload: (((result.payload ?? {}) as { approvals?: unknown[] }).approvals ?? []) as never,
    });
  }

  async function handleApproval(approvalId: string, approve: boolean) {
    if (!client) {
      return;
    }
    const feedback = approvalFeedback[approvalId]?.trim();
    await client.request(approve ? "approval_approve" : "approval_reject", {
      approval_id: approvalId,
      feedback,
    });
    await refreshApprovals();
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

  return (
    <>
      <Layout className="app-shell">
        <Layout.Sider width={320} theme="light" className="left-rail">
          <div className="rail-header">
            <Typography.Title level={4}>myclaw Operator</Typography.Title>
            <Tag color={state.connection.status === "connected" ? "green" : "gold"}>{state.connection.status}</Tag>
          </div>
          <Space.Compact style={{ width: "100%" }}>
            <Input value={endpoint} onChange={(event) => setEndpoint(event.target.value)} placeholder="ws://127.0.0.1:18080/ws" />
            <Button type="primary" onClick={connect}>
              Connect
            </Button>
          </Space.Compact>
          {state.connection.error ? <Alert type="error" showIcon message={state.connection.error} /> : null}
          {bootstrapWarnings.length > 0 ? (
            <Alert
              type="warning"
              showIcon
              message="Some myclawd methods are unavailable"
              description={bootstrapWarnings.join("; ")}
            />
          ) : null}
          <Card title="Sessions" size="small">
            <Conversations
              items={sessionItems}
              activeKey={activeSession}
              onActiveChange={() => undefined}
            />
          </Card>
          <Card title="Quick Actions" size="small">
            <Prompts
              vertical
              items={[
                { key: "read", label: "Read file", description: "Ask runtime to read a file", icon: <ToolOutlined /> },
                { key: "shell", label: "Run shell", description: "Execute shell/system command", icon: <PlayCircleOutlined /> },
                { key: "mcp", label: "Inspect MCP", description: "List servers, tools, prompts, resources", icon: <CloudServerOutlined /> },
                { key: "subagent", label: "Spawn subagent", description: "Delegate a focused task", icon: <RobotOutlined /> },
              ]}
              onItemClick={(info) => setPrompt(String(info.data.label ?? ""))}
            />
          </Card>
          <Card title="Protocol Gaps" size="small">
            <Timeline
              items={state.gaps.map((gap) => ({
                color: "gold",
                dot: <ExclamationCircleOutlined />,
                children: gap,
              }))}
            />
          </Card>
        </Layout.Sider>

        <Layout.Content className="center-pane">
          <div className="pane-header">
            <Typography.Title level={3}>Conversation Console</Typography.Title>
            <Space>
              <Tag icon={<SafetyCertificateOutlined />} color="blue">
                {state.session.permission_mode ?? "unknown permission"}
              </Tag>
              <Tag icon={<ApiOutlined />} color="purple">
                {state.session.resolved_main_loop_model ?? "model unknown"}
              </Tag>
            </Space>
          </div>

          <div className="conversation-panel">
            <Bubble.List items={transcriptItems(state.transcript, state.streaming.content)} />
            {state.connection.status !== "connected" ? (
              <div className="placeholder">
                <Spin />
                <Typography.Text type="secondary">Connect to myclawd to stream transcript and operator events.</Typography.Text>
              </div>
            ) : null}
          </div>

          <div className="composer-panel">
            <Sender
              value={prompt}
              onChange={setPrompt}
              onSubmit={sendPrompt}
              placeholder="Send prompt to myclawd"
              loading={state.connection.status === "connecting"}
            />
          </div>
        </Layout.Content>

        <Layout.Sider width={420} theme="light" className="right-rail">
          <Tabs
            defaultActiveKey="tools"
            items={[
              {
                key: "tools",
                label: "Tools",
                children: (
                  <Card size="small" title="Tool Lifecycle">
                    <Table
                      rowKey="tool_use_id"
                      size="small"
                      pagination={false}
                      dataSource={toolRows}
                      columns={[
                        {
                          title: "Tool",
                          dataIndex: "tool_name",
                          render: (value: string, record) => (
                            <Button type="link" onClick={() => setToolDrawer(record.tool_use_id)}>
                              {value}
                            </Button>
                          ),
                        },
                        {
                          title: "Status",
                          dataIndex: "status",
                          render: (value: string) => <Tag color={value === "completed" ? "green" : value === "failed" ? "red" : "blue"}>{value}</Tag>,
                        },
                      ]}
                    />
                  </Card>
                ),
              },
              {
                key: "approvals",
                label: "Approvals",
                children: (
                  <Card size="small" title="Approval Center">
                    <Space direction="vertical" style={{ width: "100%" }}>
                      {approvalRows.length === 0 ? <Alert type="info" showIcon message="No pending approvals" /> : null}
                      {approvalRows.map((approval) => (
                        <Card key={approval.approval_id} size="small">
                          <Space direction="vertical" style={{ width: "100%" }}>
                            <Typography.Text strong>{approval.tool_name ?? approval.approval_id}</Typography.Text>
                            <Typography.Text type="secondary">{approval.reason ?? "No reason provided"}</Typography.Text>
                            <Input.TextArea
                              rows={2}
                              placeholder="Optional feedback"
                              value={approvalFeedback[approval.approval_id] ?? ""}
                              onChange={(event) =>
                                setApprovalFeedback((current) => ({ ...current, [approval.approval_id]: event.target.value }))
                              }
                            />
                            <Space>
                              <Button type="primary" icon={<CheckCircleOutlined />} onClick={() => handleApproval(approval.approval_id, true)}>
                                Approve
                              </Button>
                              <Button danger onClick={() => handleApproval(approval.approval_id, false)}>
                                Reject
                              </Button>
                            </Space>
                          </Space>
                        </Card>
                      ))}
                    </Space>
                  </Card>
                ),
              },
              {
                key: "subagents",
                label: "Subagents",
                children: (
                  <Card size="small" title="Tasks / Subagents">
                    <Table
                      rowKey="run_id"
                      size="small"
                      pagination={false}
                      dataSource={subagentRows}
                      columns={[
                        { title: "Label", dataIndex: "label" },
                        { title: "Status", dataIndex: "status", render: (value: string) => <Tag>{value}</Tag> },
                        { title: "Action", dataIndex: "last_action" },
                      ]}
                    />
                  </Card>
                ),
              },
              {
                key: "mcp",
                label: "MCP",
                children: (
                  <Card size="small" title="MCP Servers / Tools / Prompts / Resources">
                    <Descriptions size="small" column={1} bordered>
                      {Object.entries(state.mcp.inventory).map(([key, value]) => (
                        <Descriptions.Item key={key} label={key}>
                          {String(value)}
                        </Descriptions.Item>
                      ))}
                    </Descriptions>
                    <Table
                      rowKey="name"
                      size="small"
                      pagination={false}
                      dataSource={state.mcp.servers}
                      columns={[
                        { title: "Server", dataIndex: "name" },
                        { title: "Status", dataIndex: "status", render: (value: string) => <Tag>{value}</Tag> },
                        { title: "Skills", dataIndex: "skills", render: (value: string[] | undefined) => (value ?? []).join(", ") || "-" },
                      ]}
                    />
                  </Card>
                ),
              },
              {
                key: "skills",
                label: "Skills",
                children: (
                  <Card size="small" title="Skills Visibility">
                    <Typography.Paragraph>
                      MCP-derived skills and runtime-observed invocations are shown here. Full catalog requires a future `skills_status` contract.
                    </Typography.Paragraph>
                    <Descriptions size="small" column={1}>
                      <Descriptions.Item label="Derived">{state.skills.derived.join(", ") || "-"}</Descriptions.Item>
                      <Descriptions.Item label="Invoked">{state.skills.invoked.join(", ") || "-"}</Descriptions.Item>
                    </Descriptions>
                  </Card>
                ),
              },
              {
                key: "runtime",
                label: "Runtime",
                children: (
                  <Card size="small" title="Runtime / Session Status">
                    <Space direction="vertical" style={{ width: "100%" }}>
                      <Descriptions size="small" column={1} bordered>
                        <Descriptions.Item label="Session ID">{state.session.session_id ?? "-"}</Descriptions.Item>
                        <Descriptions.Item label="Session Key">{state.session.session_key ?? "-"}</Descriptions.Item>
                        <Descriptions.Item label="Agent">{state.session.agent_id ?? "-"}</Descriptions.Item>
                        <Descriptions.Item label="Workspace Roots">{(state.session.workspace_roots ?? []).join(", ") || "-"}</Descriptions.Item>
                        <Descriptions.Item label="Resolved Model">{state.session.resolved_main_loop_model ?? "-"}</Descriptions.Item>
                      </Descriptions>
                      <Select
                        value={state.session.permission_mode}
                        onChange={handlePermissionMode}
                        options={[
                          { value: "default", label: "default" },
                          { value: "acceptEdits", label: "acceptEdits" },
                          { value: "bypassPermissions", label: "bypassPermissions" },
                        ]}
                      />
                      <Select
                        value={state.session.resolved_main_loop_model}
                        onChange={handleModelChange}
                        options={[
                          { value: "gpt-5.5", label: "gpt-5.5" },
                          { value: "gpt-5.4", label: "gpt-5.4" },
                          { value: "gpt-5.4-mini", label: "gpt-5.4-mini" },
                        ]}
                      />
                    </Space>
                  </Card>
                ),
              },
              {
                key: "ops",
                label: "Files / Exec",
                children: (
                  <Row gutter={[12, 12]}>
                    <Col span={24}>
                      <Card size="small" title="File Operations">
                        <Timeline
                          items={fileOps.map((tool) => ({
                            color: tool.status === "completed" ? "green" : "blue",
                            children: `${tool.tool_name}: ${tool.tool_input ?? ""}`,
                          }))}
                        />
                      </Card>
                    </Col>
                    <Col span={24}>
                      <Card size="small" title="Shell / SSH Execution">
                        <Timeline
                          items={execOps.map((tool) => ({
                            color: tool.status === "failed" ? "red" : "blue",
                            children: `${tool.tool_name}: ${tool.tool_input ?? ""}`,
                          }))}
                        />
                      </Card>
                    </Col>
                  </Row>
                ),
              },
            ]}
          />
        </Layout.Sider>
      </Layout>

      <Drawer
        open={Boolean(toolDrawer)}
        width={560}
        title={toolDrawer ? state.tools[toolDrawer]?.tool_name : "Tool Details"}
        onClose={() => setToolDrawer(null)}
      >
        {toolDrawer ? (
          <Space direction="vertical" style={{ width: "100%" }}>
            <Descriptions size="small" bordered column={1}>
              <Descriptions.Item label="Status">{state.tools[toolDrawer]?.status}</Descriptions.Item>
              <Descriptions.Item label="Input">{state.tools[toolDrawer]?.tool_input ?? "-"}</Descriptions.Item>
              <Descriptions.Item label="Run ID">{state.tools[toolDrawer]?.run_id ?? "-"}</Descriptions.Item>
            </Descriptions>
            <Card size="small" title="Progress">
              <Timeline
                items={(state.tools[toolDrawer]?.progress ?? []).map((item) => ({
                  children: `${item.type ?? "update"}: ${item.message ?? ""}`,
                }))}
              />
            </Card>
            <Card size="small" title="Structured Result">
              <pre>{JSON.stringify(state.tools[toolDrawer]?.structured_content ?? state.tools[toolDrawer]?.meta ?? {}, null, 2)}</pre>
            </Card>
          </Space>
        ) : null}
      </Drawer>
    </>
  );
}
