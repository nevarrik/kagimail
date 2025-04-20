package main

import (
	"fmt"
	"strings"

	"github.com/cloudfoundry/jibber_jabber"
	"github.com/gdamore/tcell/v2"
	"github.com/goodsign/monday"
	"github.com/rivo/tview"
)

const (
	coShortcutText       = "#ffd369"
	coHintText           = "#6c5edc"
	coMainStatusBarText  = "#ffd369"
	coEmailStatusBarText = "#ffd369"
	coEmailUnread        = "#cccccc"
	coEmailRead          = "#5c5470"

	coSelection     = "#ffd369"
	coSelectionText = "#000000"
)

func notifyFetchAllStarted(folder string, n int) {
	g_ui.app.QueueUpdateDraw(func() {
		folder := getNormalizedImapFolderName(folder)
		if g_ui.folderSelected == folder {
			return
		}
		g_ui.emailsTable.Clear()
		g_ui.emailsUidList = g_ui.emailsUidList[:0]
		g_ui.folderSelected = folder
		g_ui.folderItemCount = n
		g_ui.emailsPegSelectionToTop = true
		g_ui.previewUid = 0
		g_ui.previewText.SetTitle("Preview")
		updateStatusBar(fmt.Sprintf("Retrieving %d emails from %s", n, folder))
	})
}

func notifyFetchAllFinished(err error, folder string) {
	if err != nil {
		updateStatusBar(fmt.Sprintf(
			"Unable to download messages for folder \"%s\": %v", folder, err))
		return
	}

	g_ui.app.QueueUpdateDraw(func() {
		if g_ui.emailsTable.GetRowCount() == g_ui.folderItemCount {
			updateEmailStatusBar(fmt.Sprintf(
				"Folder up to date with %d emails", g_ui.folderItemCount))
		}
	})
}

func notifyFetchLatestStarted(folder string, n int) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.folderItemCount = n
		updateStatusBar(fmt.Sprintf("Retrieving latest emails from %s", folder))
	})
}

func notifyFetchEmailBodyStarted(folder string, uid uint32) {
	updateStatusBar(fmt.Sprintf(
		"Downloading email id: %d from %s", uid, folder))
}

const (
	notifyFetchEmailPulledFromCache = 1 << iota
)

func notifyFetchEmailBodyFinished(
	err error, folder string, uid uint32, flags uint,
) {
	if err != nil {
		updateStatusBar(fmt.Sprintf(
			"Unable to download email body for id %d: %v", uid, err))
	}

	email := cachedEmailFromUid(folder, uid)
	size := FormatHumanReadableSize(int64(email.size))
	var s string
	if flags&notifyFetchEmailPulledFromCache != 0 {
		s = fmt.Sprintf("Found cached email message: %d, size of %s", uid, size)
	} else {
		s = fmt.Sprintf("Downloaded email message: %d, size of %s", uid, size)
	}
	updateStatusBar(s)

	body := email.body
	if body == "" {
		body = fmt.Sprintf("No plaintext found, size was: %s", size)
	}
	previewPaneSetBody(uid, body)
}

func previewPaneSetBody(uid uint32, body string) {
	g_ui.app.QueueUpdateDraw(func() {
		folder := g_ui.folderSelected
		if g_ui.previewUid == uid {
			g_ui.previewText.SetText(body, false)
		}
	})
}

func previewPaneSetReply() {
	Assert(g_ui.previewUid != 0, "no preview message selected")
	Assert(g_ui.mode == UIModeQuickReply, "not in quick reply mode")

	email := cachedEmailFromUid(g_ui.folderSelected, g_ui.previewUid)

	var formattedDate string
	{ // get localized date/time formats
		userLocale, err := jibber_jabber.DetectLanguage()
		if err != nil {
			userLocale = "en_US"
		}
		locale := monday.Locale(userLocale)
		longDateFormat, ok := monday.FullFormatsByLocale[locale]
		if !ok {
			longDateFormat = monday.DefaultFormatEnUSFull
		}
		longTimeFormat, ok := monday.TimeFormatsByLocale[locale]
		if !ok {
			longTimeFormat = monday.DefaultFormatEnUSTime
		}

		formattedDate = monday.Format(
			email.date, longDateFormat+" "+longTimeFormat, locale)
	}

	var reply strings.Builder
	reply.WriteString(
		fmt.Sprintf("\n\nOn %s %s wrote:\n", formattedDate, email.fromName))

	originalText := g_ui.previewText.GetText()
	for _, line := range strings.Split(originalText, "\n") {
		line = ">" + line
		reply.WriteString(line + "\n")
	}

	g_ui.previewText.SetText(reply.String(), false)
}

func updateEmailStatusBarWithSelection() {
	if g_ui.folderItemCount == 0 {
		updateEmailStatusBar("Empty folder")
		return
	}

	k, _ := g_ui.emailsTable.GetSelection()
	updateEmailStatusBar(fmt.Sprintf(
		"Email %d of %d", k+1, g_ui.emailsTable.GetRowCount(),
	))
}

func updateEmailStatusBar(text string) {
	setFrameText := func(text string) {
		co := tcell.GetColor(coEmailStatusBarText)
		g_ui.emailsFrame.Clear().
			AddText("↑↓:Navigate", false, tview.AlignLeft, co).
			AddText(text, false, tview.AlignRight, co)
	}
	if IsOnUiThread() {
		setFrameText(text)
	} else {
		g_ui.app.QueueUpdateDraw(func() { setFrameText(text) })
	}
}

func onEmailsTableSelectionChange(row int, col int) {
	updateEmailStatusBarWithSelection()

	folder := g_ui.folderSelected
	uid := g_ui.emailsUidList[row]
	if g_ui.previewUid == uid {
		return
	}
	g_ui.previewUid = uid
	g_ui.previewText.SetTitle("Preview")
	go fetchEmailBody(g_ui.folderSelected, uid)
}

func updateStatusBar(text string) {
	text = fmt.Sprintf("[%s] %s", coMainStatusBarText, text)
	if IsOnUiThread() {
		g_ui.statusBar.SetText(text)
	} else {
		g_ui.app.QueueUpdateDraw(func() { g_ui.statusBar.SetText(text) })
	}
}

func setHintsBarText() {
	var hints string
	if g_ui.mode == UIModeNormal {
		hints = " _Compose _Reply _Forward |"
		hints += " _Quit [F5]:Refresh [Tab]:Move Focus _Hide _Preview"
	} else if g_ui.mode == UIModeQuickReply {
		hints = " [Ctrl+Enter]:Send | [Esc]:Discard"
	} else if g_ui.mode == UIModeCompose {
		hints = " [Ctrl+Enter]:Send | [Esc]:Discard"
	}

	var hintsRendered strings.Builder
	for i := 0; i < len(hints); i++ {
		if hints[i] == '_' {
			hintsRendered.WriteString(fmt.Sprintf("[%s]", coShortcutText))
			i += 1
			hintsRendered.WriteByte(hints[i])
			hintsRendered.WriteString(fmt.Sprintf("[%s]", coHintText))
			continue
		} else if hints[i] == '|' {
			hintsRendered.WriteString(fmt.Sprintf("[white]|[%s]", coHintText))
		} else if hints[i] == '[' {
			hintsRendered.WriteString(fmt.Sprintf("[%s]", coShortcutText))
		} else if hints[i] == ']' {
			hintsRendered.WriteString(fmt.Sprintf("[%s]", coHintText))
		} else {
			hintsRendered.WriteByte(hints[i])
		}
	}
	g_ui.hintsBar.SetText(hintsRendered.String())
}

func toggleHintsBar() {
	g_ui.hintsBarVisible = !g_ui.hintsBarVisible
	height := 0
	if g_ui.hintsBarVisible {
		height = 1
	}
	g_ui.emailsPane.ResizeItem(g_ui.hintsBar, height, 0)
}

func togglePreviewBar() {
	g_ui.previewVisible = !g_ui.previewVisible
	height := 0
	if g_ui.previewVisible {
		height = 5
	}
	g_ui.emailsPane.ResizeItem(g_ui.previewText, 0, height)
}

func insertImapEmailToList(email Email) {
	g_ui.app.QueueUpdateDraw(func() {
		Assert(
			g_ui.folderSelected == email.folder,
			"adding email not from selected folder",
		)

		i := cachedEmailFromUidsBinarySearch(g_ui.emailsUidList, email)
		if i < len(g_ui.emailsUidList) && g_ui.emailsUidList[i] == email.uid {
			return // already added
		}

		// insert into emailsUidList
		g_ui.emailsUidList = append(g_ui.emailsUidList, 0)
		copy(g_ui.emailsUidList[i+1:], g_ui.emailsUidList[i:])
		g_ui.emailsUidList[i] = email.uid

		// insert into table
		g_ui.emailsTable.InsertRow(i)
		co := coEmailUnread
		if email.isRead {
			co = coEmailRead
		}
		setCell := func(y int, x int, text string) {
			_, _, totalWidth, _ := g_ui.emailsTable.GetRect()
			width := []int{20, totalWidth - 20}[x]
			cell := tview.NewTableCell(text).
				SetTextColor(tcell.GetColor(co)).
				SetMaxWidth(width)
			g_ui.emailsTable.SetCell(y, x, cell)
		}
		setCell(i, 0, email.fromName)
		setCell(i, 1, email.subject)

		Assert(len(g_ui.emailsUidList) == g_ui.emailsTable.GetRowCount(), "")

		// when initially loading before keyboard input, keep top item selected
		// (items might not be inserted into ui in correct order, but our first
		// insert will set the selected item--which might then move down
		if g_ui.emailsPegSelectionToTop {
			g_ui.emailsTable.Select(0, 0)
		}

		// update statusbar
		if g_ui.emailsTable.GetRowCount() == g_ui.folderItemCount {
			updateEmailStatusBar(fmt.Sprintf(
				"Folder up to date with %d emails", g_ui.folderItemCount))
		} else {
			updateEmailStatusBar(
				fmt.Sprintf("Downloading %d emails", g_ui.folderItemCount-g_ui.emailsTable.GetRowCount()))
		}
	})
}

func removeEmailFromList(i int) {
	g_ui.emailsTable.RemoveRow(i)
	g_ui.emailsUidList = append(
		g_ui.emailsUidList[:i],
		g_ui.emailsUidList[i+1:]...)
}

func insertFolderToList(folder string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.foldersList.AddItem(
			getNormalizedImapFolderName(folder), "", 0, nil)
	})
}

func setUIMode(mode UIMode) {
	Assert(IsOnUiThread(), "won't work unless called from ui thread")
	if g_ui.mode == mode {
		return
	}

	g_ui.mode = mode
	if g_ui.mode == UIModeNormal {
		g_ui.previewText.SetTitle("Preview")
	} else if g_ui.mode == UIModeQuickReply {
		g_ui.previewText.SetTitle("Quick Reply")
	}

	if g_ui.mode == UIModeCompose {
		g_ui.pages.SwitchToPage("compose")
		g_ui.app.SetFocus(g_ui.composeForm)
	} else {
		g_ui.pages.SwitchToPage("main")
		g_ui.app.SetFocus(g_ui.emailsTable)
	}

	setHintsBarText()
}
