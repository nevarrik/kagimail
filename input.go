package main

import (
	"github.com/gdamore/tcell/v2"
)

func KeyHandler(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		fallthrough
	case tcell.KeyCtrlC:
		fallthrough
	case tcell.KeyCtrlQ:
		g_ui.app.Stop()
		return nil

	case tcell.KeyTab:
		pane := g_ui.app.GetFocus()
		if pane == g_ui.foldersPane {
			g_ui.app.SetFocus(g_ui.emailsPane)
		} else if pane == g_ui.emailsPane {
			g_ui.app.SetFocus(g_ui.previewPane)
		} else if pane == g_ui.previewPane {
			g_ui.app.SetFocus(g_ui.foldersPane)
		} else {
			AssertNotReachable("coming from a control we don't know about")
		}
		return nil

	case tcell.KeyBacktab:
		pane := g_ui.app.GetFocus()
		if pane == g_ui.foldersPane {
			g_ui.app.SetFocus(g_ui.previewPane)
		} else if pane == g_ui.emailsPane {
			g_ui.app.SetFocus(g_ui.foldersPane)
		} else if pane == g_ui.previewPane {
			g_ui.app.SetFocus(g_ui.emailsPane)
		} else {
			AssertNotReachable("coming from a control we don't know about")
		}
		return nil
	}

	g_ui.statusBar.SetText("key: " + event.Name())
	return event
}
