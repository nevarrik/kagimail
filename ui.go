package main

import (
	"fmt"
	"sort"
	"time"
)

func notifyFetchAllStarted(folder string, n int) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.emailsList.Clear()
		g_ui.emailsFolderSelected = folder
		g_ui.emailsFolderItemCount = n
		g_ui.emailsPegSelectionToTop = true
		updateStatusBar(fmt.Sprintf("retrieving %d emails from %s", n, folder))
	})
}

func notifyFetchLatestStarted(folder string, n int) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.emailsFolderItemCount = n
		updateStatusBar(fmt.Sprintf("retrieving latest emails from %s", folder))
	})
}

func previewPaneSetBody(id uint32, body string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.previewUid = id
		g_ui.previewText.SetTitle("Preview")
		g_ui.previewText.SetText(body, false)
	})
}

func updateEmailStatusBar() {
	g_ui.emailsStatusBar.SetText(fmt.Sprintf(
		"Email %d of %d (ItemCount=%d)",
		g_ui.emailsList.GetCurrentItem()+1,
		g_ui.emailsFolderItemCount,
		g_ui.emailsList.GetItemCount(),
	))
}

func updateStatusBar(text string) {
	if IsOnUiThread() {
		g_ui.statusBar.SetText(text)
	} else {
		g_ui.app.QueueUpdateDraw(func() { g_ui.statusBar.SetText(text) })
	}
}

func insertImapEmailToList(email Email) {
	g_ui.app.QueueUpdateDraw(func() {
		fnDateCompare := func(e1 Email, e2 Email) bool {
			if e1.date == e2.date {
				return e1.id > e2.id
			}
			return e1.date.After(e2.date)
		}

		// binary search and insert
		var i int
		folder := g_ui.emailsFolderSelected
		{
			g_emailsMtx.Lock()
			defer g_emailsMtx.Unlock()
			i = sort.Search(len(g_emailsFromFolder[folder]), func(k int) bool {
				return !fnDateCompare(g_emailsFromFolder[folder][k], email)
			})

			if i < len(g_emailsFromFolder[folder]) &&
				g_emailsFromFolder[folder][i].id == email.id {
				return
			}

			g_emailsFromFolder[folder] = append(
				g_emailsFromFolder[folder],
				Email{},
			)
			copy(
				g_emailsFromFolder[folder][i+1:],
				g_emailsFromFolder[folder][i:],
			)
			g_emailsFromFolder[folder][i] = email
		}

		// ui uses i to match g_emails order
		g_ui.emailsList.InsertItem(
			i,
			email.subject,
			fmt.Sprintf(
				"%s from: %s",
				email.date.Format(time.Stamp),
				email.fromAddress,
			),
			0,
			func() { go fetchEmailBody(folder, email.id) },
		)

		if g_ui.emailsPegSelectionToTop {
			g_ui.emailsList.SetCurrentItem(0)
		}

		if g_ui.emailsList.GetItemCount() == g_ui.emailsFolderItemCount {
			g_ui.emailsStatusBar.SetText(fmt.Sprintf(
				"folder up to date with %d emails", g_ui.emailsFolderItemCount))
		} else {
			g_ui.emailsStatusBar.SetText(
				fmt.Sprintf("downloading %d emails", g_ui.emailsFolderItemCount-g_ui.emailsList.GetItemCount()))
		}
	})
}

func insertFolderToList(mailbox string) {
	g_ui.app.QueueUpdateDraw(func() {
		if mailbox == "INBOX" {
			mailbox = "Inbox"
		}

		g_ui.foldersList.AddItem(mailbox, "", 0,
			func() { go fetchFolder(mailbox) })
	})
}
