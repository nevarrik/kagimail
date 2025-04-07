package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ui struct {
	app         *tview.Application
	foldersPane *tview.TextView
	emailsPane  *tview.List
	previewPane *tview.TextView
	statusBar   *tview.TextView

	columnsPane *tview.Flex
	mainPane    *tview.Flex
}

var g_ui ui

func main() {
	g_ui.app = tview.NewApplication()

	g_ui.foldersPane = tview.NewTextView()
	g_ui.emailsPane = tview.NewList()
	g_ui.previewPane = tview.NewTextView()
	g_ui.statusBar = tview.NewTextView()

	g_ui.foldersPane.SetText("inbox").
		SetTextColor(tcell.ColorWhite).
		SetBorder(true).
		SetTitle("Folders")

	g_ui.emailsPane.
		SetBorder(true).
		SetTitle("Emails")

	// emails := [][]string{
	// 	{"Please pay", "You owe us two dollars, I want them", "Gym"},
	// 	{"How are you?", "Haven't heard from you in while", "Stu"},
	// 	{"Travel plans", "Are you coming down to Hong Kong with us?", "Lloyd"},
	// 	{"Subscription Newsletter", "The latest news", "Kagi"},
	// }

	// for y, email := range emails {
	// 	for x, col := range email {
	// 		cell := tview.NewTableCell(col)
	// 		emailsPane.SetCell(y, x, cell)
	// 	}
	// }

	g_ui.emailsPane.AddItem("Please pay", "from: Gym", 'a', nil)
	g_ui.emailsPane.AddItem("How are you?", "from: Stu", 'b', nil)
	g_ui.emailsPane.AddItem("Travel plans", "from: Lloyd", 'c', nil)
	g_ui.emailsPane.AddItem("Newsletters", "from: Kagi", 'd', nil)

	g_ui.previewPane.SetText(
		"Hey buddy, how is everything going?\nSincerely,\n\nJames").
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

	err := g_ui.app.Run()
	if err != nil {
		panic(err)
	}
}
