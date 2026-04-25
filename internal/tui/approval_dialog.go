package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type approvalDialogAction int

const (
	approvalDialogNoop approvalDialogAction = iota
	approvalDialogApprove
	approvalDialogReject
)

type approvalDialogResult struct {
	Action approvalDialogAction
}

type approvalDialogState struct {
	Request       *clientApproval
	SelectedIndex int
}

func newApprovalDialogState() approvalDialogState {
	return approvalDialogState{}
}

func (d approvalDialogState) active() bool {
	return d.Request != nil
}

func (d *approvalDialogState) open(request *clientApproval) {
	if request == nil {
		d.close()
		return
	}
	copied := *request
	d.Request = &copied
	d.SelectedIndex = 0
}

func (d *approvalDialogState) close() {
	*d = newApprovalDialogState()
}

func (d *approvalDialogState) moveUp() {
	if d.SelectedIndex > 0 {
		d.SelectedIndex--
	}
}

func (d *approvalDialogState) moveDown() {
	if d.SelectedIndex < 1 {
		d.SelectedIndex++
	}
}

func (d *approvalDialogState) handleKey(msg tea.KeyMsg) approvalDialogResult {
	if !d.active() {
		return approvalDialogResult{}
	}
	switch msg.Type {
	case tea.KeyEnter:
		if d.SelectedIndex == 0 {
			return approvalDialogResult{Action: approvalDialogApprove}
		}
		return approvalDialogResult{Action: approvalDialogReject}
	case tea.KeyCtrlY:
		return approvalDialogResult{Action: approvalDialogApprove}
	case tea.KeyEscape, tea.KeyCtrlN:
		return approvalDialogResult{Action: approvalDialogReject}
	case tea.KeyUp, tea.KeyCtrlP:
		d.moveUp()
	case tea.KeyDown:
		d.moveDown()
	case tea.KeyRunes:
		switch strings.TrimSpace(strings.ToLower(string(msg.Runes))) {
		case "y":
			return approvalDialogResult{Action: approvalDialogApprove}
		case "n":
			return approvalDialogResult{Action: approvalDialogReject}
		}
	}
	return approvalDialogResult{}
}
