package orchestration

import "context"

type Event struct {
	Type       string
	SessionID  string
	SessionKey string
	AgentID    string
	RunID      string
	ToolName   string
	ToolInput  string
	Status     string
	Action     string
	Message    string
}

type Hook interface {
	Handle(context.Context, Event) error
}

type Chain []Hook

func (c Chain) Handle(ctx context.Context, event Event) error {
	for _, hook := range c {
		if hook == nil {
			continue
		}
		if err := hook.Handle(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
