package main

import (
	"log"
	"os"

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
	g_ui.foldersList.SetSelectedFunc(
		func(_ int, main string, _ string, _ rune) {
			go fetchFolder(main, fetchFolderOptionAllEmails)
		},
	)

	g_ui.emailsTable = tview.NewTable()
	g_ui.emailsTable.SetSelectable(true, false)
	g_ui.emailsTable.SetSelectionChangedFunc(onEmailsTableSelectionChange)

	g_ui.previewText = tview.NewTextArea()
	g_ui.hintsBar = tview.NewTextView()
	g_ui.hintsBar.SetDynamicColors(true)
	setHintsBarText()

	g_ui.statusBar = tview.NewTextView()
	g_ui.statusBar.SetDynamicColors(true)

	g_ui.foldersList.
		SetHighlightFullLine(true).
		ShowSecondaryText(false).
		SetBorder(true).
		SetTitle("Folders")

	g_ui.previewText.
		SetBorder(true).
		SetTitle("Preview")

	g_ui.emailsFrame = tview.NewFrame(g_ui.emailsTable).
		SetBorders(0, 0, 1, 0, 1, 1)
	updateEmailStatusBarWithSelection()
	g_ui.emailsPane = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(g_ui.hintsBar, 0, 0, false).
		AddItem(g_ui.emailsFrame, 0, 7, false).
		AddItem(g_ui.previewText, 0, 0, false)
	toggleHintsBar()
	togglePreviewBar()

	g_ui.columnsPane = tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(g_ui.foldersList, 0, 0, false).
		AddItem(g_ui.emailsPane, 0, 9, false)
	toggleFoldersPane()

	g_ui.mainPane = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(g_ui.columnsPane, 0, 10, false).
		AddItem(g_ui.statusBar, 1, 0, false)

	g_ui.pages = tview.NewPages()
	g_ui.pages.AddPage("main", g_ui.mainPane, true, true)

	g_ui.composeForm = tview.NewForm()
	g_ui.composeForm.
		SetBorder(true).
		SetTitle("Compose").
		SetTitleAlign(tview.AlignLeft)
	g_ui.composeForm.AddInputField("To:", "", 0, nil, nil)
	g_ui.composeForm.AddInputField("Cc:", "", 0, nil, nil)
	g_ui.composeForm.AddInputField("Subject:", "", 0, nil, nil)
	g_ui.composeForm.AddTextArea("Message:", "", 0, 12, 0, nil)

	g_ui.composePane = tview.NewFlex()
	g_ui.composePane.SetDirection(tview.FlexRow).
		AddItem(g_ui.hintsBar, 1, 0, false).
		AddItem(g_ui.composeForm, 0, 1, true)
	g_ui.pages.AddPage("compose", g_ui.composePane, true, false)

	g_ui.app.SetInputCapture(KeyHandler)

	g_ui.app.SetRoot(g_ui.pages, true)
	g_ui.app.SetFocus(g_ui.emailsTable)
	onFocusChange()

	go imapInit()
	go smtpInit()

	err = g_ui.app.Run()
	if err != nil {
		panic(err)
	}
}
