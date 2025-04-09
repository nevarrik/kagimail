package main

import (
	"github.com/rivo/tview"
)

func main() {
	g_ui.app = tview.NewApplication()

	g_ui.foldersPane = tview.NewList()
	g_ui.emailsPane = tview.NewList()
	g_ui.previewPane = tview.NewTextView()
	g_ui.statusBar = tview.NewTextView()

	g_ui.foldersPane.
		SetHighlightFullLine(true).
		ShowSecondaryText(false).
		SetBorder(true).
		SetTitle("Folders")

	g_ui.emailsPane.
		SetBorder(true).
		SetTitle("Emails")

	g_ui.previewPane.
		SetBorder(true).
		SetTitle("Preview")

	g_ui.statusBar.SetText("status: good")

	g_ui.columnsPane = tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(g_ui.foldersPane, 0, 1, false).
		AddItem(g_ui.emailsPane, 0, 4, false).AddItem(g_ui.previewPane, 0, 5, false)

	g_ui.mainPane = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(g_ui.columnsPane, 0, 10, false).
		AddItem(g_ui.statusBar, 1, 0, false)

	g_ui.app.SetInputCapture(KeyHandler)
	g_ui.app.SetRoot(g_ui.mainPane, true)
	g_ui.app.SetFocus(g_ui.emailsPane)

	go imapInit()

	err := g_ui.app.Run()
	if err != nil {
		panic(err)
	}
}
