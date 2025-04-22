package main

import (
	"context"
	"log"

	"github.com/rivo/tview"
)

const (
	UIModeNormal = iota
	UIModeQuickReply
	UIModeCompose
)

type UIMode int

type UI struct {
	app   *tview.Application
	mode  UIMode
	pages *tview.Pages

	// main pane
	//
	foldersList        *tview.List
	foldersListVisible bool

	emailsFrame          *tview.Frame
	emailsTable          *tview.Table
	emailsUidList        []uint32
	folderSelected       string
	folderItemCount      int
	folderDownloadCancel context.CancelFunc
	// we set this when we begin downloading all emails from a folder, to keep
	// the first element selected until they manually change the selection
	emailsPegSelectionToTop bool

	previewText    *tview.TextArea
	previewUid     uint32
	previewVisible bool

	hintsBar        *tview.TextView
	hintsBarVisible bool
	statusBar       *tview.TextView
	columnsPane     *tview.Flex
	mainPane        *tview.Flex
	emailsPane      *tview.Flex

	// compose pane
	//
	composePane *tview.Flex
	composeForm *tview.Form
}

type MailConfig struct {
	IMAPHost    string `toml:"imap_host"`
	SMTPHost    string `toml:"smtp_host"`
	Email       string `toml:"email"`
	Password    string `toml:"password"`
	DisplayName string `toml:"display_name"`
	Trace       bool   `toml:"trace,omitempty"`
}

var (
	g_ui     UI
	g_config MailConfig
)

func trace(format string, v ...any) {
	if g_config.Trace {
		log.Printf(format, v...)
	}
}
