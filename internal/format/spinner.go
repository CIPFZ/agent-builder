package format

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Spinner is a lightweight stderr progress indicator for non-interactive runs.
type Spinner struct {
	ctx       context.Context
	stop      context.CancelFunc
	label     string
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewSpinner creates a non-TUI spinner.
func NewSpinner(ctx context.Context, _ context.CancelFunc, label string) *Spinner {
	spinnerCtx, stop := context.WithCancel(ctx)
	return &Spinner{
		ctx:   spinnerCtx,
		stop:  stop,
		label: label,
		done:  make(chan struct{}),
	}
}

// Start begins the spinner animation.
func (s *Spinner) Start() {
	s.startOnce.Do(func() {
		go func() {
			defer close(s.done)
			ticker := time.NewTicker(120 * time.Millisecond)
			defer ticker.Stop()

			frames := []string{"|", "/", "-", "\\"}
			i := 0
			for {
				select {
				case <-s.ctx.Done():
					fmt.Fprint(os.Stderr, ansi.EraseEntireLine)
					return
				case <-ticker.C:
					fmt.Fprintf(os.Stderr, "\r%s %s...", frames[i%len(frames)], s.label)
					i++
				}
			}
		}()
	})
}

// Stop ends the spinner animation.
func (s *Spinner) Stop() {
	s.stopOnce.Do(func() {
		s.stop()
		<-s.done
		fmt.Fprint(os.Stderr, "\r"+ansi.EraseEntireLine)
	})
}
