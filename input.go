package main

import (
	"github.com/gdamore/tcell/v2"
)

func KeyHandler(event *tcell.EventKey) *tcell.EventKey {
	pane := g_ui.app.GetFocus()
	mode := g_ui.mode

	allowSingleKeys := mode != UIModeQuickReply
	if allowSingleKeys && event.Key() == tcell.KeyRune {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'r':
				if g_ui.previewUid != 0 {
					setUIMode(UIModeQuickReply)
					previewPaneSetReply()
					g_ui.app.SetFocus(g_ui.previewText)
				} else {
					updateStatusBar("No message selected to reply to")
				}
				return nil

			case 'h':
				toggleHintsBar()
				return nil

			case 'q':
				g_ui.app.Stop()
				return nil
			}
		}
	}

	if mode == UIModeQuickReply && pane == g_ui.previewText {
		switch event.Key() {
		case tcell.KeyCtrlJ: // this is sent on ^enter
			email := cachedEmailFromUid(
				g_ui.folderSelected, g_ui.previewUid)
			replyEmail(email, g_ui.previewText.GetText())
			setUIMode(UIModeNormal)
			g_ui.previewText.SetText("", false)
			g_ui.app.SetFocus(g_ui.emailsList)
			return nil

		case tcell.KeyEscape:
			setUIMode(UIModeNormal)
			g_ui.previewText.SetText("", false)
			g_ui.app.SetFocus(g_ui.emailsList)
			return nil
		}
	}

	if pane == g_ui.emailsList {
		switch event.Key() {
		case tcell.KeyUp,
			tcell.KeyDown,
			tcell.KeyHome,
			tcell.KeyEnd,
			tcell.KeyPgUp,
			tcell.KeyPgDn,
			tcell.KeyEnter:
			g_ui.emailsPegSelectionToTop = false
		}
	}

	switch event.Key() {
	case tcell.KeyCtrlC:
		fallthrough
	case tcell.KeyCtrlQ:
		g_ui.app.Stop()
		return nil

	case tcell.KeyTab:
		if pane == g_ui.foldersList {
			g_ui.app.SetFocus(g_ui.emailsList)
		} else if pane == g_ui.emailsList {
			g_ui.app.SetFocus(g_ui.previewText)
		} else if pane == g_ui.previewText {
			g_ui.app.SetFocus(g_ui.foldersList)
		} else {
			AssertNotReachable("coming from a control we don't know about")
		}
		return nil

	case tcell.KeyBacktab:
		if pane == g_ui.foldersList {
			g_ui.app.SetFocus(g_ui.previewText)
		} else if pane == g_ui.emailsList {
			g_ui.app.SetFocus(g_ui.foldersList)
		} else if pane == g_ui.previewText {
			g_ui.app.SetFocus(g_ui.emailsList)
		} else {
			AssertNotReachable("coming from a control we don't know about")
		}
		return nil
	}

	return event
}
