package main

import (
	"log"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	f, err := os.OpenFile(
		"kagimail.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o644,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	log.SetOutput(f)

	modelInit()

	g_ui.app = tview.NewApplication()

	g_ui.foldersList = tview.NewList()

	g_ui.emailsList = tview.NewList()
	g_ui.emailsList.SetWrapAround(false)
	g_ui.emailsList.SetChangedFunc(
		func(int, string, string, rune) { updateEmailStatusBar() })
	g_ui.emailsList.SetSelectedFunc(func(k int, _ string, _ string, _ rune) {
		go fetchEmailBody(g_ui.folderSelected, g_ui.emailsUidList[k])
	})
	g_ui.emailsStatusBar = tview.NewTextView()

	g_ui.previewText = tview.NewTextArea()
	g_ui.statusBar = tview.NewTextView()

	g_ui.foldersList.
		SetHighlightFullLine(true).
		ShowSecondaryText(false).
		SetBorder(true).
		SetTitle("Folders")

	g_ui.emailsList.
		SetBorder(true).
		SetTitle("Emails")

	g_ui.previewText.
		SetBorder(true).
		SetTitle("Preview")

	g_ui.emailsStatusBar.
		SetTextAlign(tview.AlignRight).
		SetText("Downloading emails ").
		SetTextColor(tcell.NewHexColor(0xFFD369)).
		SetBackgroundColor(tcell.NewHexColor(0x393E46))

	g_ui.emailsPane = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(g_ui.emailsList, 0, 10, true).
		AddItem(g_ui.emailsStatusBar, 1, 0, false)

	g_ui.columnsPane = tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(g_ui.foldersList, 0, 1, false).
		AddItem(g_ui.emailsPane, 0, 4, false).
		AddItem(g_ui.previewText, 0, 5, false)

	g_ui.mainPane = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(g_ui.columnsPane, 0, 10, false).
		AddItem(g_ui.statusBar, 1, 0, false)

	g_ui.app.SetInputCapture(KeyHandler)
	g_ui.app.SetRoot(g_ui.mainPane, true)
	g_ui.app.SetFocus(g_ui.emailsList)

	go imapInit()
	go smtpInit()

	err = g_ui.app.Run()
	if err != nil {
		panic(err)
	}
}
