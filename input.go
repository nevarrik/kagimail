package main

import (
	"github.com/gdamore/tcell/v2"
)

const (
	PreviewMode = 1 << iota
	QuickReplyMode
	ReplyMode
)

func KeyHandler(event *tcell.EventKey) *tcell.EventKey {
	pane := g_ui.app.GetFocus()
	mode := 0
	if g_ui.previewText.GetTitle() == "Preview" {
		mode = PreviewMode
	} else if g_ui.previewText.GetTitle() == "Quick Reply" {
		mode = QuickReplyMode
	}

	inEmailsOrPreview := pane == g_ui.emailsList || pane == g_ui.previewText
	if mode == PreviewMode && inEmailsOrPreview {
		if (event.Key() == tcell.KeyRune && event.Rune() == 'r') ||
			event.Key() == tcell.KeyCtrlR {
			previewPaneSetReply()
			return nil
		}

		switch event.Key() {
		}
	}

	if mode == QuickReplyMode && inEmailsOrPreview {
		if event.Key() == tcell.KeyCtrlJ { // this is sent on ^enter
			email := cachedEmailFromUid(
				g_ui.folderSelected, g_ui.previewUid)
			replyEmail(email, g_ui.previewText.GetText())

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
	case tcell.KeyEscape:
		fallthrough
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
