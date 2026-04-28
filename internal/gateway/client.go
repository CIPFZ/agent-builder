package gateway

import (
	"sync"

	"github.com/gorilla/websocket"

	protocolws "myclaw/internal/protocol/ws"
)

type Client struct {
	id                        string
	clientIdentity            string
	sessionID                 string
	sessionKey                string
	supportsPermissionControl bool
	pendingControlResponses   map[string]chan protocolws.Message
	conn                      *websocket.Conn
	mu                        sync.Mutex
}

func NewClient(id string, conn *websocket.Conn) *Client {
	return &Client{
		id:   id,
		conn: conn,
	}
}

func (c *Client) ID() string {
	return c.id
}

func (c *Client) BindIdentity(identity string) {
	c.clientIdentity = identity
}

func (c *Client) Identity() string {
	return c.clientIdentity
}

func (c *Client) BindSession(sessionID, sessionKey string) {
	c.sessionID = sessionID
	c.sessionKey = sessionKey
}

func (c *Client) SetSupportsPermissionControl(supported bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.supportsPermissionControl = supported
}

func (c *Client) SupportsPermissionControl() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.supportsPermissionControl
}

func (c *Client) SessionID() string {
	return c.sessionID
}

func (c *Client) SessionKey() string {
	return c.sessionKey
}

func (c *Client) WriteJSON(msg protocolws.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn.WriteJSON(msg)
}

func (c *Client) RegisterControlRequest(id string) <-chan protocolws.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pendingControlResponses == nil {
		c.pendingControlResponses = make(map[string]chan protocolws.Message)
	}
	ch := make(chan protocolws.Message, 1)
	c.pendingControlResponses[id] = ch
	return ch
}

func (c *Client) ResolveControlResponse(msg protocolws.Message) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pendingControlResponses == nil {
		return false
	}
	ch, ok := c.pendingControlResponses[msg.ID]
	if !ok {
		return false
	}
	delete(c.pendingControlResponses, msg.ID)
	ch <- msg
	close(ch)
	return true
}

func (c *Client) CancelControlRequest(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.pendingControlResponses == nil {
		return
	}
	delete(c.pendingControlResponses, id)
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn.Close()
}
