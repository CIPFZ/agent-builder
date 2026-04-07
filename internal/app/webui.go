package app

import "fmt"

func webUIHTML(wsPath string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>MyClaw Control Panel</title>
  <style>
    :root {
      --bg: #f5efe4;
      --paper: #fffaf0;
      --ink: #1d2a2a;
      --muted: #5d6b6b;
      --line: #d8c9ad;
      --accent: #8d3f2b;
      --accent-2: #2d6a5c;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: Georgia, "Noto Serif SC", serif;
      color: var(--ink);
      background:
        radial-gradient(circle at top left, #fff7df 0, transparent 35%%),
        linear-gradient(135deg, #f5efe4, #efe5d0);
    }
    .shell {
      max-width: 1200px;
      margin: 0 auto;
      padding: 24px;
      display: grid;
      gap: 16px;
    }
    .hero, .panel {
      background: rgba(255,250,240,.92);
      border: 1px solid var(--line);
      border-radius: 18px;
      box-shadow: 0 12px 40px rgba(29,42,42,.08);
    }
    .hero { padding: 24px; }
    .hero h1 { margin: 0 0 8px; font-size: 36px; }
    .hero p { margin: 0; color: var(--muted); }
    .grid {
      display: grid;
      gap: 16px;
      grid-template-columns: 340px minmax(0, 1fr);
    }
    .panel { padding: 16px; }
    .panel h2 {
      margin: 0 0 12px;
      font-size: 18px;
    }
    .stack { display: grid; gap: 10px; }
    label {
      display: grid;
      gap: 6px;
      font-size: 13px;
      color: var(--muted);
    }
    input, textarea, button, select {
      font: inherit;
    }
    input, textarea, select {
      width: 100%%;
      padding: 10px 12px;
      border-radius: 12px;
      border: 1px solid var(--line);
      background: #fffdf8;
      color: var(--ink);
    }
    textarea { min-height: 96px; resize: vertical; }
    .row {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
    }
    button {
      border: 0;
      border-radius: 999px;
      padding: 10px 16px;
      background: var(--accent);
      color: #fff8ef;
      cursor: pointer;
    }
    button.alt {
      background: var(--accent-2);
    }
    button.ghost {
      background: transparent;
      color: var(--accent);
      border: 1px solid var(--line);
    }
    .status {
      display: flex;
      gap: 10px;
      align-items: center;
      flex-wrap: wrap;
      font-size: 14px;
    }
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 6px 10px;
      border-radius: 999px;
      background: #f1e3cb;
    }
    .dot {
      width: 10px;
      height: 10px;
      border-radius: 999px;
      background: #b74d2c;
    }
    .dot.live { background: #2d6a5c; }
    .console {
      min-height: 540px;
      max-height: 70vh;
      overflow: auto;
      padding: 12px;
      border-radius: 14px;
      background: #1d2a2a;
      color: #f7f3e8;
      font-family: "Cascadia Code", Consolas, monospace;
      font-size: 13px;
      white-space: pre-wrap;
    }
    .meta {
      color: #d5c7a8;
      margin-bottom: 8px;
    }
    .kv {
      display: grid;
      gap: 8px;
      padding: 12px;
      border-radius: 14px;
      background: #f7efdf;
      border: 1px solid var(--line);
      font-size: 13px;
    }
    .kv code {
      word-break: break-all;
    }
    .list {
      display: grid;
      gap: 8px;
      margin: 0;
      padding: 0;
      list-style: none;
    }
    .list button {
      width: 100%%;
      text-align: left;
      border-radius: 14px;
    }
    .transcript {
      display: grid;
      gap: 10px;
      margin: 12px 0 0;
      padding: 0;
      list-style: none;
    }
    .bubble {
      padding: 12px;
      border-radius: 14px;
      border: 1px solid var(--line);
      background: #fffdf8;
    }
    .bubble.assistant {
      background: #f2f0e8;
      border-color: #c7d4cd;
    }
    .bubble .role {
      font-size: 12px;
      color: var(--muted);
      margin-bottom: 6px;
      text-transform: uppercase;
      letter-spacing: .08em;
    }
    .hint {
      font-size: 12px;
      color: var(--muted);
    }
    @media (max-width: 920px) {
      .grid { grid-template-columns: 1fr; }
      .hero h1 { font-size: 28px; }
    }
  </style>
</head>
<body>
  <div class="shell">
    <section class="hero">
      <h1>MyClaw Control Panel</h1>
      <p>Connect to the daemon, send a message, inspect events, and trigger runtime controls from one page.</p>
    </section>
    <section class="grid">
      <aside class="panel stack">
        <div class="status">
          <span class="badge"><span id="status-dot" class="dot"></span><span id="status-text">Disconnected</span></span>
          <span class="badge">WS <code id="ws-path">%s</code></span>
        </div>
        <label>
          Client Identity
          <input id="client-identity" value="web-ui">
        </label>
        <label>
          Agent ID
          <input id="agent-id" value="main">
        </label>
        <div class="row">
          <button id="connect-btn">Connect</button>
          <button id="disconnect-btn" class="ghost">Disconnect</button>
        </div>
        <label>
          Message
          <textarea id="message-input" placeholder="Try: hello or tool run pwd"></textarea>
        </label>
        <div class="row">
          <button id="send-btn" class="alt">Send Message</button>
          <button id="plan-btn" class="ghost">Get Plan</button>
          <button id="history-btn" class="ghost">Get History</button>
        </div>
        <h2>Session Status</h2>
        <div class="row">
          <button id="session-btn" class="ghost">Get Session Status</button>
        </div>
        <div class="kv">
          <div>Session ID: <code id="session-id-value">not connected</code></div>
          <div>Session Key: <code id="session-key-value">not connected</code></div>
          <div>Permission Mode: <code id="permission-mode-value">unknown</code></div>
        </div>
        <label>
          Approval ID
          <input id="approval-id" placeholder="paste approval_id here">
        </label>
        <h2>Approval Actions</h2>
        <div class="row">
          <button id="approval-list-btn" class="ghost">List Approvals</button>
          <button id="approval-approve-btn" class="alt">Approve</button>
          <button id="approval-reject-btn" class="ghost">Reject</button>
        </div>
        <ul id="approval-items" class="list"></ul>
        <p class="hint">The page auto-issues a connect request after the socket opens. Events and responses stream into the console.</p>
      </aside>
      <main class="panel">
        <h2>Quick View</h2>
        <div class="kv">
          <div>Plan Summary: <code id="plan-summary-value">not loaded</code></div>
          <div>History Summary: <code id="history-summary-value">not loaded</code></div>
        </div>
        <h2>Transcript</h2>
        <ul id="transcript-items" class="transcript"></ul>
        <h2>Event Stream</h2>
        <div id="console" class="console"></div>
      </main>
    </section>
  </div>
  <script>
    const WS_PATH = %q;
    const consoleEl = document.getElementById("console");
    const statusDot = document.getElementById("status-dot");
    const statusText = document.getElementById("status-text");
    const clientIdentity = document.getElementById("client-identity");
    const agentId = document.getElementById("agent-id");
    const messageInput = document.getElementById("message-input");
    const approvalID = document.getElementById("approval-id");
    const sessionIDValue = document.getElementById("session-id-value");
    const sessionKeyValue = document.getElementById("session-key-value");
    const permissionModeValue = document.getElementById("permission-mode-value");
    const approvalItems = document.getElementById("approval-items");
    const planSummaryValue = document.getElementById("plan-summary-value");
    const historySummaryValue = document.getElementById("history-summary-value");
    const transcriptItems = document.getElementById("transcript-items");
    let socket = null;
    let requestID = 0;
    let lastApprovals = [];
    let pendingAssistantBubble = null;

    function nextID() {
      requestID += 1;
      return String(requestID);
    }

    function append(kind, payload) {
      const time = new Date().toLocaleTimeString();
      const meta = document.createElement("div");
      meta.className = "meta";
      meta.textContent = "[" + time + "] " + kind;
      const body = document.createElement("div");
      body.textContent = typeof payload === "string" ? payload : JSON.stringify(payload, null, 2);
      consoleEl.append(meta, body, document.createTextNode("\n"));
      consoleEl.scrollTop = consoleEl.scrollHeight;
    }

    function setConnected(connected) {
      statusDot.classList.toggle("live", connected);
      statusText.textContent = connected ? "Connected" : "Disconnected";
    }

    function updateSessionInfo(payload) {
      if (!payload || typeof payload !== "object") return;
      if (payload.session_id) sessionIDValue.textContent = payload.session_id;
      if (payload.session_key) sessionKeyValue.textContent = payload.session_key;
      if (payload.permission_mode) permissionModeValue.textContent = payload.permission_mode;
    }

    function renderApprovals(items) {
      lastApprovals = Array.isArray(items) ? items : [];
      approvalItems.innerHTML = "";
      if (!lastApprovals.length) {
        const empty = document.createElement("li");
        empty.textContent = "No approvals loaded";
        approvalItems.appendChild(empty);
        return;
      }
      lastApprovals.forEach((item) => {
        const li = document.createElement("li");
        const button = document.createElement("button");
        button.className = "ghost";
        button.textContent = (item.id || "unknown") + " | " + (item.status || "unknown") + " | " + (item.reason || "no reason");
        button.addEventListener("click", () => {
          approvalID.value = item.id || "";
        });
        li.appendChild(button);
        approvalItems.appendChild(li);
      });
    }

    function renderPlanSummary(payload) {
      if (!payload || typeof payload !== "object") return;
      const steps = Array.isArray(payload.steps) ? payload.steps.length : 0;
      const groups = payload.groups && typeof payload.groups === "object" ? Object.keys(payload.groups).length : 0;
      planSummaryValue.textContent = "steps=" + steps + ", groups=" + groups;
    }

    function renderHistorySummary(summary) {
      if (!summary || typeof summary !== "object") return;
      const count = summary.record_count || 0;
      const latest = summary.latest_recorded_action || "none";
      historySummaryValue.textContent = "records=" + count + ", latest=" + latest;
    }

    function renderTranscriptMessage(message) {
      if (!message || typeof message !== "object") return;
      if (message.role === "assistant" && pendingAssistantBubble) {
        pendingAssistantBubble.remove();
        pendingAssistantBubble = null;
      }
      const item = document.createElement("li");
      const bubble = document.createElement("div");
      const role = document.createElement("div");
      const content = document.createElement("div");
      bubble.className = "bubble " + (message.role || "unknown");
      role.className = "role";
      role.textContent = message.role || "unknown";
      content.textContent = message.content || "";
      bubble.append(role, content);
      item.appendChild(bubble);
      transcriptItems.appendChild(item);
    }

    function applyAssistantDelta(delta) {
      if (!delta) return;
      if (!pendingAssistantBubble) {
        const item = document.createElement("li");
        const bubble = document.createElement("div");
        const role = document.createElement("div");
        const content = document.createElement("div");
        bubble.className = "bubble assistant";
        role.className = "role";
        role.textContent = "assistant";
        content.textContent = "";
        bubble.append(role, content);
        item.appendChild(bubble);
        transcriptItems.appendChild(item);
        pendingAssistantBubble = bubble;
      }
      const content = pendingAssistantBubble.lastElementChild;
      if (content) {
        content.textContent = (content.textContent || "") + delta;
      }
    }

    function wsURL() {
      const proto = location.protocol === "https:" ? "wss://" : "ws://";
      return proto + location.host + WS_PATH;
    }

    function send(method, payload) {
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        append("client.error", "WebSocket is not connected");
        return;
      }
          const message = { type: "req", id: nextID(), method, payload: payload || {} };
      socket.send(JSON.stringify(message));
      append("client.request " + method, message);
    }

    function connect() {
      if (socket && socket.readyState === WebSocket.OPEN) {
        append("client.info", "Already connected");
        return;
      }
      socket = new WebSocket(wsURL());
      append("client.info", "Connecting to " + wsURL());
      socket.addEventListener("open", () => {
        setConnected(true);
        send("connect", {
          role: "client",
          client_identity: clientIdentity.value.trim() || "web-ui",
          agent_id: agentId.value.trim() || "main"
        });
      });
      socket.addEventListener("message", (event) => {
        try {
          const payload = JSON.parse(event.data);
          append("server.message", payload);
          if (payload && payload.ok && payload.payload) {
            updateSessionInfo(payload.payload);
            if (Array.isArray(payload.payload.approvals)) {
              renderApprovals(payload.payload.approvals);
            }
            if (Array.isArray(payload.payload.steps)) {
              renderPlanSummary(payload.payload);
            }
            if (payload.payload.summary && typeof payload.payload.summary === "object") {
              renderHistorySummary(payload.payload.summary);
            }
          }
          if (payload && payload.type === "event" && payload.event === "hello" && payload.payload) {
            updateSessionInfo(payload.payload);
          }
          if (payload && payload.type === "event" && payload.event === "message.created" && payload.payload && payload.payload.message) {
            renderTranscriptMessage(payload.payload.message);
          }
          if (payload && payload.type === "event" && payload.event === "assistant.delta" && payload.payload && payload.payload.delta) {
            applyAssistantDelta(payload.payload.delta);
          }
          if (payload && payload.type === "event" && payload.event === "permission.required" && payload.payload && payload.payload.approval_id) {
            approvalID.value = payload.payload.approval_id;
          }
          if (payload && payload.type === "event" && payload.event === "approval.updated" && payload.payload && payload.payload.approval_id) {
            approvalID.value = payload.payload.approval_id;
          }
        } catch (err) {
          append("server.message", event.data);
        }
      });
      socket.addEventListener("close", () => {
        setConnected(false);
        append("client.info", "Socket closed");
      });
      socket.addEventListener("error", () => {
        append("client.error", "Socket error");
      });
    }

    document.getElementById("connect-btn").addEventListener("click", connect);
    document.getElementById("disconnect-btn").addEventListener("click", () => {
      if (socket) socket.close();
    });
    document.getElementById("send-btn").addEventListener("click", () => {
      send("send_message", { content: messageInput.value });
    });
    document.getElementById("plan-btn").addEventListener("click", () => {
      send("orchestration_plan", {});
    });
    document.getElementById("history-btn").addEventListener("click", () => {
      send("orchestration_plan_execution_history", {});
    });
    document.getElementById("session-btn").addEventListener("click", () => {
      send("session_status", {});
    });
    document.getElementById("approval-list-btn").addEventListener("click", () => {
      send("approval_list", {});
    });
    document.getElementById("approval-approve-btn").addEventListener("click", () => {
      send("approval_approve", { approval_id: approvalID.value.trim() });
    });
    document.getElementById("approval-reject-btn").addEventListener("click", () => {
      send("approval_reject", { approval_id: approvalID.value.trim() });
    });
  </script>
</body>
</html>`, wsPath, wsPath)
}
