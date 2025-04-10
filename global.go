package main

import (
	"sync"

	"github.com/rivo/tview"
)

type UI struct {
	app         *tview.Application
	foldersPane *tview.List
	emailsPane  *tview.List
	previewPane *tview.TextArea
	previewUid  uint32
	statusBar   *tview.TextView

	columnsPane *tview.Flex
	mainPane    *tview.Flex
}

type MailConfig struct {
	IMAPHost    string `toml:"imap_host"`
	SMTPHost    string `toml:"smtp_host"`
	Email       string `toml:"email"`
	Password    string `toml:"password"`
	DisplayName string `toml:"display_name"`
}

var (
	g_ui     UI
	g_config MailConfig

	g_emailsMtx sync.Mutex
	g_emailsTbl map[uint32]Email
)
