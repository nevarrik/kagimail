package main

import (
	"fmt"
	"sort"
	"time"
)

func notifyFetchStarted(folder string, n int) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.emailsFolderItemCount = n
		g_ui.emailsStatusBar.SetText(fmt.Sprintf("retrieving %d emails from %s",
			n, folder))
	})
}

func previewPaneSetBody(id uint32, body string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.previewUid = id
		g_ui.previewPane.SetTitle("Preview")
		g_ui.previewPane.SetText(body, false)
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

		// binary search and insert
		g_emailsMtx.Lock()
		i := sort.Search(len(g_emailsFromFolder), func(k int) bool {
			return !fnDateCompare(g_emailsFromFolder[k], email)
		})

		g_emailsFromFolder = append(g_emailsFromFolder, Email{})
		copy(g_emailsFromFolder[i+1:], g_emailsFromFolder[i:])
		g_emailsFromFolder[i] = email
		g_emailsMtx.Unlock()

		// ui uses i to match g_emails order
		g_ui.emailsList.InsertItem(
			i,
			email.subject,
			fmt.Sprintf(
				"%s from: %s",
				email.date.Format(time.RFC3339),
				email.fromAddress,
			),
			0,
			func() {
				go grabEmail(email.id)
			},
		)
	})
}

func insertFolderToList(mailbox string) {
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.foldersPane.AddItem(mailbox, "", 0,
			func() {
				g_ui.emailsList.Clear()
				grabLatestEmails(mailbox)
			})
	})
}
