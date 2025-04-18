package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudfoundry/jibber_jabber"
	"github.com/goodsign/monday"
)

const (
	coShortcutText  = "#ffd369"
	coHintText      = "#fb2576"
	coStatusBarText = "#6eacda"
)

func notifyFetchAllStarted(folder string, n int) {
	g_ui.app.QueueUpdateDraw(func() {
		folder := getNormalizedImapFolderName(folder)
		if g_ui.folderSelected == folder {
			return
		}
		g_ui.emailsList.Clear()
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
		if g_ui.emailsList.GetItemCount() == g_ui.folderItemCount {
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

func previewPaneSetBody(id uint32, body string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.previewUid = id
		g_ui.previewText.SetTitle("Preview")
		g_ui.previewText.SetText(body, false)
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
	k := g_ui.emailsList.GetCurrentItem()
	updateEmailStatusBar(fmt.Sprintf(
		"Email %d of %d [%d] (ItemCount=%d)",
		k+1,
		g_ui.folderItemCount,
		g_ui.emailsUidList[k],
		g_ui.emailsList.GetItemCount(),
	))
}

func updateEmailStatusBar(text string) {
	if IsOnUiThread() {
		g_ui.emailsStatusBar.SetText(text)
	} else {
		g_ui.app.QueueUpdateDraw(func() { g_ui.emailsStatusBar.SetText(text) })
	}
}

func updateStatusBar(text string) {
	text = fmt.Sprintf("[%s] %s", coStatusBarText, text)
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
		hints += " _Quit [F5]:Refresh [Tab]:Move Focus _Hide"
	} else if g_ui.mode == UIModeQuickReply {
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
	g_ui.mainPane.ResizeItem(g_ui.hintsBar, height, 0)
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
		// insert into ui
		secondaryLine := fmt.Sprintf("%s from: %s",
			email.date.Format(time.Stamp), email.fromAddress)
		g_ui.emailsList.InsertItem(
			i, email.subject, secondaryLine, 0, nil)

		Assert(len(g_ui.emailsUidList) == g_ui.emailsList.GetItemCount(), "")

		// when initially loading before keyboard input, keep top item selected
		// (items might not be inserted into ui in correct order, but our first
		// insert will set the selected item--which might then move down
		if g_ui.emailsPegSelectionToTop {
			g_ui.emailsList.SetCurrentItem(0)
		}

		// update statusbar
		if g_ui.emailsList.GetItemCount() == g_ui.folderItemCount {
			updateEmailStatusBar(fmt.Sprintf(
				"Folder up to date with %d emails", g_ui.folderItemCount))
		} else {
			updateEmailStatusBar(
				fmt.Sprintf("Downloading %d emails", g_ui.folderItemCount-g_ui.emailsList.GetItemCount()))
		}
	})
}

func removeEmailFromList(i int) {
	g_ui.emailsList.RemoveItem(i)
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

	setHintsBarText()
}
