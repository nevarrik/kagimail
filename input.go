package main

import "github.com/gdamore/tcell/v2"

func KeyHandler(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlC ||
		event.Key() == tcell.KeyCtrlQ {
		g_ui.app.Stop()
		return nil
	}

	g_ui.statusBar.SetText("key: " + event.Name())
	return event
}
