package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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
	switch keyEventType(msg) {
	case keyEnter:
		if d.SelectedIndex == 0 {
			return approvalDialogResult{Action: approvalDialogApprove}
		}
		return approvalDialogResult{Action: approvalDialogReject}
	case keyCtrlY:
		return approvalDialogResult{Action: approvalDialogApprove}
	case keyEscape, keyCtrlN:
		return approvalDialogResult{Action: approvalDialogReject}
	case keyUp, keyCtrlP:
		d.moveUp()
	case keyDown:
		d.moveDown()
	case keyRunes:
		switch strings.TrimSpace(strings.ToLower(string(keyEventRunes(msg)))) {
		case "y":
			return approvalDialogResult{Action: approvalDialogApprove}
		case "n":
			return approvalDialogResult{Action: approvalDialogReject}
		}
	}
	return approvalDialogResult{}
}
