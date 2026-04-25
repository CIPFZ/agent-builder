package tui

import tea "charm.land/bubbletea/v2"

func testKey(event keyEvent) tea.KeyMsg {
	key := tea.Key{}
	switch event {
	case keyEnter:
		key.Code = tea.KeyEnter
	case keyEscape:
		key.Code = tea.KeyEscape
	case keyCtrlC:
		key.Code, key.Mod = 'c', tea.ModCtrl
	case keyCtrlY:
		key.Code, key.Mod = 'y', tea.ModCtrl
	case keyCtrlN:
		key.Code, key.Mod = 'n', tea.ModCtrl
	case keyCtrlP:
		key.Code, key.Mod = 'p', tea.ModCtrl
	case keyCtrlR:
		key.Code, key.Mod = 'r', tea.ModCtrl
	case keyCtrlK:
		key.Code, key.Mod = 'k', tea.ModCtrl
	case keyCtrlO:
		key.Code, key.Mod = 'o', tea.ModCtrl
	case keyCtrlF:
		key.Code, key.Mod = 'f', tea.ModCtrl
	case keyCtrlE:
		key.Code, key.Mod = 'e', tea.ModCtrl
	case keyCtrlG:
		key.Code, key.Mod = 'g', tea.ModCtrl
	case keyCtrlX:
		key.Code, key.Mod = 'x', tea.ModCtrl
	case keyCtrlS:
		key.Code, key.Mod = 's', tea.ModCtrl
	case keyUp:
		key.Code = tea.KeyUp
	case keyDown:
		key.Code = tea.KeyDown
	case keyLeft:
		key.Code = tea.KeyLeft
	case keyRight:
		key.Code = tea.KeyRight
	case keyPgUp:
		key.Code = tea.KeyPgUp
	case keyPgDown:
		key.Code = tea.KeyPgDown
	case keyHome:
		key.Code = tea.KeyHome
	case keyEnd:
		key.Code = tea.KeyEnd
	case keyTab:
		key.Code = tea.KeyTab
	case keyShiftTab:
		key.Code, key.Mod = tea.KeyTab, tea.ModShift
	case keyShiftUp:
		key.Code, key.Mod = tea.KeyUp, tea.ModShift
	case keyShiftDown:
		key.Code, key.Mod = tea.KeyDown, tea.ModShift
	case keyBackspace:
		key.Code = tea.KeyBackspace
	case keyDelete:
		key.Code = tea.KeyDelete
	case keySpace:
		key.Code = tea.KeySpace
	default:
		key.Code = 0
	}
	return tea.KeyPressMsg(key)
}

func testKeyRunes(text string) tea.KeyMsg {
	return tea.KeyPressMsg(tea.Key{Text: text, Code: []rune(text)[0]})
}

func testMouseWheel(button tea.MouseButton) tea.MouseMsg {
	return tea.MouseWheelMsg(tea.Mouse{Button: button})
}
