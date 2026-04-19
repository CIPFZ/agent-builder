package tui

import "strings"

const dialogKindCompaction = "compaction"

func (m *Model) acceptCompactionItem(item dialogItem) {
	switch item.Value {
	case "microcompact":
		result, err := m.bridge.MicrocompactSession()
		if err != nil {
			m.applyBridgeError(err)
			return
		}
		m.applyCompactionActionResult("compact.micro", result)
	default:
		if instructions, ok := trimCompactionValue(item.Value); ok {
			result, err := m.bridge.CompactSession(instructions)
			if err != nil {
				m.applyBridgeError(err)
				return
			}
			m.applyCompactionActionResult("compact.manual", result)
		}
	}
}

func (m *Model) applyCompactionActionResult(event string, result compactionActionResult) {
	m.events = appendBoundedEvent(m.events, event, 200)
	if result.Changed {
		m.busy = false
		m.activity.Label = "Compaction completed"
		m.transcript = append(m.transcript, transcriptEntry{
			Kind:    messageKindCompact,
			Role:    "system",
			Content: "Conversation compacted",
		})
		m.noteTranscriptAppended()
		return
	}
	m.busy = false
	m.activity.Label = "Compaction not needed"
}

func trimCompactionValue(value string) (string, bool) {
	const prefix = "compact:"
	if strings.HasPrefix(value, prefix) {
		return strings.TrimSpace(value[len(prefix):]), true
	}
	return "", false
}
