# Terminal Production Architecture

Status: implementation gate for the production terminal path.

## Goals

The right-panel terminal must behave like a real integrated terminal, not a
command runner. The production path is:

```text
xterm.js renderer
  <-> terminal WebSocket stream
  <-> Go runtime terminal host
  <-> PTY/ConPTY
  <-> shell process
```

React owns layout and tab state only. Go owns shell lifecycle, terminal
profiles, PTY/ConPTY handles, process cleanup, and bounded replay history.

## Technology Choices

### Renderer

Use `@xterm/xterm` with `@xterm/addon-fit`.

Rules:

- Do not render prompt, command rows, or output rows in React.
- Feed user input from `terminal.onData` to the runtime stream.
- Feed PTY output to `terminal.write`.
- Write output serially and acknowledge only after xterm has accepted the
  write callback.
- Keep scrollback bounded.

### Runtime Terminal Host

Use a Go runtime terminal host backed by `github.com/aymanbagabas/go-pty`.

Rules:

- One runtime terminal session owns one shell process.
- Windows uses ConPTY through the PTY abstraction; Unix-like systems use a PTY.
- WebSocket disconnect does not kill the shell.
- Tab close explicitly closes the runtime terminal and kills the process.
- App shutdown closes all terminal sessions.
- Replay history is byte bounded; very large output must stream, not accumulate
  without limit.

### Transport

Use a terminal-scoped WebSocket as the main production transport:

```text
GET /v1/terminals/{terminal_id}/stream?token=...
```

Terminal input, resize, output, ack, and close control use this WebSocket
stream. The first product version does not keep old request/response terminal
input or SSE terminal output compatibility endpoints.

Reasons:

- Terminal traffic is low-latency, high-frequency, bidirectional byte flow.
- Browser dev and Wails desktop can share one transport.
- Wails remains an adapter and does not become the terminal business boundary.
- The protocol can be contract-tested without launching the desktop shell.

## WebSocket Protocol

Client to server:

```json
{"type":"input","data":"..."}
{"type":"resize","columns":120,"rows":32}
{"type":"ack","sequence":42}
{"type":"close"}
```

Server to client:

```json
{"type":"output","events":[{"sequence":42,"data":"..."}]}
{"type":"final","events":[{"sequence":43,"final":true,"status":"exited"}]}
{"type":"error","error":"..."}
```

Output messages may contain multiple terminal events. The server batches by
small time windows and size thresholds. The client acknowledges the highest
sequence after xterm has written the corresponding output. The server uses ack
to apply backpressure; it must not generate an unbounded connection queue.

## Backpressure

Backpressure is required because shell output can exceed xterm renderer
throughput.

Runtime rules:

- PTY reads publish bounded replay events.
- Subscriber queues are bounded.
- A full subscriber queue applies short backpressure and then removes the stale
  subscriber instead of dropping output silently.
- The WebSocket writer sends a bounded batch and waits for ack before sending
  the next batch.

Frontend rules:

- xterm writes are serialized.
- The frontend sends ack only after `terminal.write(data, callback)` completes.
- Input remains low latency and is sent in small batches.
- Resize is debounced.

## Large Output

The terminal must handle output larger than 10 MB without unbounded memory
growth.

Expected behavior:

- Stream output continuously.
- Keep xterm scrollback bounded.
- Keep runtime replay history bounded by bytes.
- If a client reconnects after falling behind the replay window, it may lose
  old terminal display history but the shell process remains authoritative.
- Full durable capture of huge command output belongs in artifacts/log files,
  not terminal scrollback.

## Connection Management

- WebSocket connect attaches to an existing terminal session.
- WebSocket disconnect detaches only.
- Reconnect uses `after=N` to replay bounded history.
- Tab close calls runtime terminal delete/close.
- Final process events close subscribers.
- Runtime exposes terminal status, rows/columns, shell profile, and exit code.

## Non-goals

- Do not make React the source of terminal lifecycle.
- Do not infer shell state from terminal text.
- Do not store unbounded terminal output in Go or browser memory.
- Do not use request/response command execution as the main terminal model.
