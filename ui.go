package main

import (
	"fmt"
	"sort"
	"time"
)

func notifyFetchStarted(folder string, n int) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.emailsList.Clear()
		g_ui.emailsFolderSelected = folder
		g_ui.emailsFolderItemCount = n
		g_ui.emailsPegSelectionToTop = true
		updateStatusBar(fmt.Sprintf("retrieving %d emails from %s", n, folder))
	})
}

func previewPaneSetBody(id uint32, body string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.previewUid = id
		g_ui.previewText.SetTitle("Preview")
		g_ui.previewText.SetText(body, false)
	})
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
		g_ui.emailsFolderItemDownloaded++
		if g_ui.emailsFolderItemDownloaded == g_ui.emailsFolderItemCount {
			g_ui.emailsStatusBar.SetText(
				fmt.Sprintf(
					"finished downloading %d emails",
					g_ui.emailsFolderItemCount,
				),
			)
		} else {
			percentage := float32(g_ui.emailsFolderItemDownloaded) / float32(g_ui.emailsFolderItemCount) * 100.0
			g_ui.emailsStatusBar.SetText(
				fmt.Sprintf("downloaded %d of %d emails, (%.0f%%) list: %d", g_ui.emailsFolderItemDownloaded, g_ui.emailsFolderItemCount, percentage, g_ui.emailsList.GetItemCount()))
		}

		fnDateCompare := func(e1 Email, e2 Email) bool {
			return e1.date.After(e2.date)
		}

		folder := g_ui.emailsFolderSelected
		// binary search and insert
		g_emailsMtx.Lock()
		i := sort.Search(len(g_emailsFromFolder[folder]), func(k int) bool {
			return !fnDateCompare(g_emailsFromFolder[folder][k], email)
		})

		g_emailsFromFolder[folder] = append(g_emailsFromFolder[folder], Email{})
		copy(g_emailsFromFolder[folder][i+1:], g_emailsFromFolder[folder][i:])
		g_emailsFromFolder[folder][i] = email
		g_emailsMtx.Unlock()

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
