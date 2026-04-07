package runtime

import (
	"context"
	"sync"

	"myclaw/internal/session"
)

type Queue struct {
	runner *Runner

	mu       sync.Mutex
	queues   map[string]chan queuedTask
	inflight map[string]int
}

type queuedTask struct {
	session session.Session
	message session.Message
	sink    EventSink
}

func NewQueue(runner *Runner) *Queue {
	return &Queue{
		runner:   runner,
		queues:   make(map[string]chan queuedTask),
		inflight: make(map[string]int),
	}
}

func (q *Queue) Enqueue(ctx context.Context, sess session.Session, msg session.Message, sink EventSink) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	ch, ok := q.queues[sess.ID]
	if !ok {
		ch = make(chan queuedTask, 64)
		q.queues[sess.ID] = ch
		go q.runSessionQueue(ch)
	}

	q.inflight[sess.ID]++
	pending := q.inflight[sess.ID]
	ch <- queuedTask{
		session: sess,
		message: msg,
		sink:    sink,
	}

	return pending
}

func (q *Queue) runSessionQueue(ch chan queuedTask) {
	for task := range ch {
		_ = q.runner.HandleUserMessage(context.Background(), task.session, task.message, task.sink)

		q.mu.Lock()
		q.inflight[task.session.ID]--
		q.mu.Unlock()
	}
}
