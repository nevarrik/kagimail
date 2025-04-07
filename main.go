package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func main() {
	app := tview.NewApplication()

	statusBar := tview.NewTextView().SetText("status: good")

	foldersPane := tview.NewTextView()
	emailsPane := tview.NewTextView()
	previewPane := tview.NewTextView()

	foldersPane.SetText("inbox").
		SetTextColor(tcell.ColorWhite).
		SetBorder(true).
		SetTitle("Folders")

	emailsPane.SetText("Subject").
		SetBorder(true).
		SetTitle("Emails")
	previewPane.SetText(
		"Hey buddy, how is everything going?\nSincerely,\n\nJames").
		SetBorder(true).
		SetTitle("Preview")

	columnsPane := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(foldersPane, 0, 1, false).
		AddItem(emailsPane, 0, 4, false).AddItem(previewPane, 0, 5, false)

	mainPane := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(columnsPane, 0, 10, false).
		AddItem(statusBar, 1, 0, false)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlC ||
			event.Key() == tcell.KeyCtrlQ {
			app.Stop()
			return nil
		}

		app.QueueUpdateDraw(func() {
			statusBar.SetText("key: " + event.Name())
		})
		return event
	})

	err := app.SetRoot(mainPane, true).Run()
	if err != nil {
		panic(err)
	}
}
