package gateway

import (
	"sync"

	"github.com/gorilla/websocket"

	protocolws "myclaw/internal/protocol/ws"
)

type Client struct {
	id         string
	sessionID  string
	sessionKey string
	conn       *websocket.Conn
	mu         sync.Mutex
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

func (c *Client) BindSession(sessionID, sessionKey string) {
	c.sessionID = sessionID
	c.sessionKey = sessionKey
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

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.conn.Close()
}
