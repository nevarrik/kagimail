package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudfoundry/jibber_jabber"
	"github.com/gdamore/tcell/v2"
	"github.com/goodsign/monday"
	"github.com/rivo/tview"
)

const (
	coKagiYellow = "#ffd369"
	coKagiPurple = "#6c5edc"

	coShortcutText       = coKagiYellow
	coHintText           = coKagiPurple
	coMainStatusBarText  = coKagiYellow
	coEmailStatusBarText = coKagiYellow
	coEmailUnread        = "#cccccc"
	coEmailRead          = "#5c5470"

	coSelectionFocused      = coKagiPurple
	coSelectionTextFocused  = "#000000"
	coSelectionInactive     = "#bbbbbb"
	coSelectionTextInactive = "#000000"
)

func notifyFetchStarted(folder string, n int, cancel context.CancelFunc) {
	Assert(IsOnUiThread(), "modifying g_ui should be on ui thread")
	g_ui.folderItemCount = n
	g_ui.folderDownloadCancel = cancel
	updateStatusBar(fmt.Sprintf("Retrieving latest emails from %s", folder))
}

func notifyFetchAllStarted(folder string, n int, cancel context.CancelFunc) {
	g_ui.app.QueueUpdateDraw(func() {
		folder := getNormalizedImapFolderName(folder)
		notifyFetchStarted(folder, n, cancel)
		if g_ui.folderSelected == folder {
			return
		}
		// switching folders initialization
		g_ui.emailsTable.Clear()
		g_ui.emailsUidList = g_ui.emailsUidList[:0]
		g_ui.folderSelected = folder
		trace("g_ui.emailsPegSelectionToTop set")
		g_ui.emailsPegSelectionToTop = true
		g_ui.previewUid = 0
		g_ui.previewText.SetTitle("Preview")
	})
}

func notifyFetchAllFinished(err error, folder string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.folderDownloadCancel = nil
		updateEmailStatusBarWithSelection()
		if err != nil {
			if err == context.Canceled {
				updateStatusBar("Cancelled downloading of folder")
			} else {
				updateStatusBar(fmt.Sprintf(
					"Unable to download messages for folder \"%s\": %v",
					folder, err))
			}
		} else {
			Assert(
				g_ui.emailsTable.GetRowCount() == g_ui.folderItemCount,
				"notifyFetchAllFinished should have downloaded everything",
			)
			updateStatusBar(fmt.Sprintf(
				"Folder up to date with %d emails as of %s", g_ui.folderItemCount,
				time.Now().Format(time.Stamp),
			))
		}
	})
}

func notifyFetchLatestStarted(folder string, n int, cancel context.CancelFunc) {
	g_ui.app.QueueUpdateDraw(func() {
		notifyFetchStarted(folder, n, cancel)
	})
}

func notifyFetchLatestFinished(err error, folder string) {
	notifyFetchAllFinished(err, folder)
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
		email := cachedEmailFromUid(folder, uid)
		trace(
			"previewPaneSetBody came in for email: %d, %s",
			uid,
			email.subject,
		)
		if g_ui.previewUid == uid {
			g_ui.previewText.SetText(body, false)
			// if we are in UIModeQuickReply, that means they hit r on this
			// message, but it hadn't downloaded yet. But now that we have it
			// go ahead transform the body into a reply and set focus
			if g_ui.mode == UIModeQuickReply {
				previewPaneSetReply()
			}
		}
	})
}

func previewPaneSetReply() {
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
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
	g_ui.app.SetFocus(g_ui.previewText)
	onFocusChange()
}

func updateEmailStatusBarWithSelection() {
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
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
		Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
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
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
	updateEmailStatusBarWithSelection()

	folder := g_ui.folderSelected
	uid := g_ui.emailsUidList[row]
	if g_ui.previewUid == uid {
		return
	}

	email := cachedEmailFromUid(folder, uid)
	trace(
		"emailsTable.selection change to row:%d, col:%d, email seq:%d, uid:%d, %s",
		row,
		col,
		email.seqNum,
		email.uid,
		email.subject,
	)

	g_ui.previewUid = uid
	g_ui.previewText.SetTitle("Preview")
	g_ui.previewText.SetText("", false)
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
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
	var hints string
	if g_ui.mode == UIModeNormal {
		hints = " _Compose _Reply _Forward [F5]:Refresh |"
		hints += " [Tab]:Move Focus _Folders _Hints _Preview _Quit"
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

func toggleFoldersPane() {
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
	g_ui.foldersListVisible = !g_ui.foldersListVisible
	width := 0
	if g_ui.foldersListVisible {
		width = 2
	}
	g_ui.columnsPane.ResizeItem(g_ui.foldersList, 0, width)
}

func toggleHintsBar() {
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
	g_ui.hintsBarVisible = !g_ui.hintsBarVisible
	height := 0
	if g_ui.hintsBarVisible {
		height = 1
	}
	g_ui.emailsPane.ResizeItem(g_ui.hintsBar, height, 0)
}

func togglePreviewBar() {
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
	g_ui.previewVisible = !g_ui.previewVisible
	height := 0
	if g_ui.previewVisible {
		height = 5
	}
	g_ui.emailsPane.ResizeItem(g_ui.previewText, 0, height)
}

const (
	insertImapEmailOptionRestore uint32 = 1 << iota
	insertImapEmailOptionDownload
)

func insertImapEmailToList(email Email, insertImapEmailOptionFlags uint32) {
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
		if email.fromName != "" {
			setCell(i, 0, email.fromName)
		} else {
			setCell(i, 0, email.fromAddress)
		}
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
			updateEmailStatusBarWithSelection()
		} else {
			verb := "Downloading"
			if insertImapEmailOptionFlags&insertImapEmailOptionRestore != 0 {
				verb = "Loading"
			}
			updateEmailStatusBar(
				fmt.Sprintf("%s %d emails", verb, g_ui.folderItemCount-g_ui.emailsTable.GetRowCount()))
		}
	})
}

func removeEmailFromList(i int) {
	Assert(IsOnUiThread(), "g_ui access should be syncronized on ui thread")
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
		email := cachedEmailFromUid(g_ui.folderSelected, g_ui.previewUid)
		g_ui.previewText.SetTitle("Replying to " + email.fromName)
	}

	if g_ui.mode == UIModeCompose {
		g_ui.pages.SwitchToPage("compose")
		g_ui.app.SetFocus(g_ui.composeForm)
	} else {
		g_ui.pages.SwitchToPage("main")
		g_ui.app.SetFocus(g_ui.emailsTable)
	}

	onFocusChange()
	setHintsBarText()
}

func onFocusChange() {
	Assert(IsOnUiThread(), "won't work unless called from ui thread")

	makeSelectionStyle := func(coBk, coFg string) tcell.Style {
		return tcell.StyleDefault.
			Background(tcell.GetColor(coBk)).
			Foreground(tcell.GetColor(coFg))
	}

	selectionStyleFocused := makeSelectionStyle(
		coSelectionFocused, coSelectionTextFocused)
	selectionStyleInactive := makeSelectionStyle(
		coSelectionInactive, coSelectionTextInactive)
	coBorderFocused := tcell.GetColor(coSelectionFocused)
	coBorderInactive := tcell.GetColor(coSelectionInactive)

	emailsSelectionStyle := selectionStyleFocused
	if !g_ui.emailsTable.HasFocus() {
		emailsSelectionStyle = selectionStyleInactive
	}

	foldersSelectionStyle := selectionStyleFocused
	foldersBorderColor := coBorderFocused
	if !g_ui.foldersList.HasFocus() {
		foldersBorderColor = coBorderInactive
		foldersSelectionStyle = selectionStyleInactive
	}

	previewBorderColor := coBorderFocused
	if !g_ui.previewText.HasFocus() {
		previewBorderColor = coBorderInactive
	}

	g_ui.emailsTable.SetSelectedStyle(emailsSelectionStyle)

	g_ui.foldersList.SetBorderColor(foldersBorderColor)
	g_ui.foldersList.SetTitleColor(foldersBorderColor)
	g_ui.foldersList.SetSelectedStyle(foldersSelectionStyle)

	g_ui.previewText.SetBorderColor(previewBorderColor)
	g_ui.previewText.SetTitleColor(previewBorderColor)

	g_ui.app.ForceDraw()
}
