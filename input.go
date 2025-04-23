package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func KeyHandler(event *tcell.EventKey) *tcell.EventKey {
	pane := g_ui.app.GetFocus()
	mode := g_ui.mode

	allowSingleKeys := mode == UIModeNormal
	if allowSingleKeys {
		if event.Key() == tcell.KeyEsc {
			if g_ui.folderDownloadCancel != nil {
				g_ui.folderDownloadCancel()
			}
		}

		if event.Key() == tcell.KeyF5 {
			go fetchFolder(g_ui.folderSelected, fetchFolderOptionLatestOnly)
			return nil
		}

		ctrlLetter := event.Modifiers()&tcell.ModCtrl != 0

		if event.Key() == tcell.KeyRune || ctrlLetter {
			char := event.Rune()
			if ctrlLetter {
				// convert ctrl + alpha into rune, by taking its rune value
				// which is the ascii value as a control character
				// e.g. Ctrl+A == 65 == 'A'; Ctrl+H == 8 == Backspace == 'H'
				char = 'a' + event.Rune() - 1
			}

			switch char {
			case 'r':
				if g_ui.previewUid != 0 {
					if g_ui.previewVisible == false {
						togglePreviewBar()
					}
					setUIMode(UIModeQuickReply)
					previewPaneSetReply()
				} else {
					updateStatusBar("No message selected to reply to")
				}
				return nil

			case 'f':
				setUIMode(UIModeCompose)
				composeSetForward()
				return nil

			case 'd':
				toggleFoldersPane()
				return nil

			case 'h':
				toggleHintsBar()
				return nil

			case 'p':
				togglePreviewBar()
				return nil

			case 'c':
				setUIMode(UIModeCompose)
				composeClear()
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
			row, _ := g_ui.emailsTable.GetSelection()
			g_ui.emailsTable.Select(max(0, row-1), 0)
			g_ui.app.SetFocus(g_ui.emailsTable)
			return nil

		case tcell.KeyEscape:
			setUIMode(UIModeNormal)
			g_ui.previewText.SetText("", false)
			g_ui.app.SetFocus(g_ui.emailsTable)
			return nil
		}
	}

	if mode == UIModeCompose {
		switch event.Key() {
		case tcell.KeyCtrlJ:
			fnGetFormItem := g_ui.composeForm.GetFormItem
			email := Email{
				toAddress: fnGetFormItem(0).(*tview.InputField).GetText(),
				ccAddress: fnGetFormItem(1).(*tview.InputField).GetText(),
				subject:   fnGetFormItem(2).(*tview.InputField).GetText(),
				body:      fnGetFormItem(3).(*tview.TextArea).GetText(),
			}
			setUIMode(UIModeNormal)
			composeEmail(email)

			fnGetFormItem(0).(*tview.InputField).SetText("")
			fnGetFormItem(1).(*tview.InputField).SetText("")
			fnGetFormItem(2).(*tview.InputField).SetText("")
			fnGetFormItem(3).(*tview.TextArea).SetText("", true)
			return nil

		case tcell.KeyEsc:
			setUIMode(UIModeNormal)
			return nil
		}
	}

	// peg selections to top workaround
	if pane == g_ui.emailsTable {
		switch event.Key() {
		case tcell.KeyUp,
			tcell.KeyDown,
			tcell.KeyHome,
			tcell.KeyEnd,
			tcell.KeyPgUp,
			tcell.KeyPgDn,
			tcell.KeyEnter:
			trace("g_ui.emailsPegSelectionToTop cleared")
			g_ui.emailsPegSelectionToTop = false
		}
	}

	// global keys
	switch event.Key() {
	case tcell.KeyCtrlC:
		fallthrough
	case tcell.KeyCtrlQ:
		g_ui.app.Stop()
		return nil
	}

	// focus moving tabbing keys
	if mode == UIModeNormal {
		switch event.Key() {
		case tcell.KeyTab:
			if pane == g_ui.foldersList {
				g_ui.app.SetFocus(g_ui.emailsTable)
			} else if pane == g_ui.emailsTable {
				g_ui.app.SetFocus(g_ui.previewText)
			} else if pane == g_ui.previewText {
				g_ui.app.SetFocus(g_ui.foldersList)
			} else {
				AssertNotReachable("coming from a control we don't know about")
			}

			onFocusChange()
			return nil

		case tcell.KeyBacktab:
			if pane == g_ui.foldersList {
				g_ui.app.SetFocus(g_ui.previewText)
			} else if pane == g_ui.emailsTable {
				g_ui.app.SetFocus(g_ui.foldersList)
			} else if pane == g_ui.previewText {
				g_ui.app.SetFocus(g_ui.emailsTable)
			} else {
				AssertNotReachable("coming from a control we don't know about")
			}

			onFocusChange()
			return nil
		}
	}

	return event
}
