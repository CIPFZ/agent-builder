package tui

import tea "charm.land/bubbletea/v2"

type keyEvent int

const (
	keyUnknown keyEvent = iota
	keyRunes
	keyEnter
	keyEscape
	keyCtrlC
	keyCtrlY
	keyCtrlN
	keyCtrlP
	keyCtrlR
	keyCtrlK
	keyCtrlO
	keyCtrlF
	keyCtrlE
	keyCtrlG
	keyCtrlX
	keyCtrlS
	keyUp
	keyDown
	keyLeft
	keyRight
	keyPgUp
	keyPgDown
	keyHome
	keyEnd
	keyTab
	keyShiftTab
	keyShiftUp
	keyShiftDown
	keyBackspace
	keyDelete
	keySpace
)

func keyEventType(msg tea.KeyMsg) keyEvent {
	key := msg.Key()
	if key.Mod&tea.ModCtrl != 0 {
		switch key.Code {
		case 'c':
			return keyCtrlC
		case 'y':
			return keyCtrlY
		case 'n':
			return keyCtrlN
		case 'p':
			return keyCtrlP
		case 'r':
			return keyCtrlR
		case 'k':
			return keyCtrlK
		case 'o':
			return keyCtrlO
		case 'f':
			return keyCtrlF
		case 'e':
			return keyCtrlE
		case 'g':
			return keyCtrlG
		case 'x':
			return keyCtrlX
		case 's':
			return keyCtrlS
		}
	}
	if key.Mod&tea.ModShift != 0 {
		switch key.Code {
		case tea.KeyTab:
			return keyShiftTab
		case tea.KeyUp:
			return keyShiftUp
		case tea.KeyDown:
			return keyShiftDown
		}
	}
	switch key.Code {
	case tea.KeyEnter, tea.KeyKpEnter:
		return keyEnter
	case tea.KeyEscape:
		return keyEscape
	case tea.KeyUp:
		return keyUp
	case tea.KeyDown:
		return keyDown
	case tea.KeyLeft:
		return keyLeft
	case tea.KeyRight:
		return keyRight
	case tea.KeyPgUp:
		return keyPgUp
	case tea.KeyPgDown:
		return keyPgDown
	case tea.KeyHome:
		return keyHome
	case tea.KeyEnd:
		return keyEnd
	case tea.KeyTab:
		return keyTab
	case tea.KeyBackspace:
		return keyBackspace
	case tea.KeyDelete:
		return keyDelete
	case tea.KeySpace:
		return keySpace
	}
	if key.Text != "" {
		return keyRunes
	}
	return keyUnknown
}

func keyEventRunes(msg tea.KeyMsg) []rune {
	return []rune(msg.Key().Text)
}
