package main

import (
	"sync"

	"github.com/rivo/tview"
)

type UI struct {
	app         *tview.Application
	foldersPane *tview.List
	emailsPane  *tview.List
	previewPane *tview.TextView
	statusBar   *tview.TextView

	columnsPane *tview.Flex
	mainPane    *tview.Flex
}

var (
	g_ui     UI
	g_config IMAPConfig

	g_emailsMtx sync.Mutex
	g_emails    []Email
)
