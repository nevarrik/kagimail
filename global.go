package main

import (
	"sync"

	"github.com/rivo/tview"
)

type UI struct {
	app                        *tview.Application
	foldersPane                *tview.List
	emailsList                 *tview.List
	emailsStatusBar            *tview.TextView
	emailsFolderSelected       string
	emailsFolderItemCount      int
	emailsFolderItemDownloaded int
	previewPane                *tview.TextArea
	previewUid                 uint32
	statusBar                  *tview.TextView

	columnsPane *tview.Flex
	mainPane    *tview.Flex
	emailsPane  *tview.Flex
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

	g_emails    []Email
	g_emailsMtx                 sync.Mutex
	g_emailFromUid              map[uint32]Email
)
