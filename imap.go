package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

type Email struct {
	id          uint32
	subject     string
	date        time.Time
	toAddress   string
	fromAddress string
	fromName    string
	body        string
}

var (
	chCheckForNewMessages chan string
	chDownloadEmailByUid  chan uint32
	chDownloadFolders     chan bool
)

func grabEmail(id uint32) {
	chDownloadEmailByUid <- id
}

func grabLatestEmails(folder string) {
	chCheckForNewMessages <- folder
}

func imapInit() {
	chCheckForNewMessages = make(chan string, 10)
	chDownloadEmailByUid = make(chan uint32, 10)
	chDownloadFolders = make(chan bool, 1)

	go imapWorker()
	grabLatestEmails("inbox")
	chDownloadFolders <- true
}

func imapWorker() {
	_, err := toml.DecodeFile("kagimail.toml", &g_config)
	if err != nil {
		log.Fatal(err)
	}

	clt, err := client.DialTLS(g_config.IMAPHost+":993", nil)
	if err != nil {
		log.Fatal(err)
	}

	err = clt.Login(g_config.Email, g_config.Password)
	defer clt.Logout()
	if err != nil {
		log.Fatal(err)
	}

	const (
		FetchBodyViaUID = 1 << iota
		FecthLast10Emails
	)

	fetchEmail := func(
		uid uint32, folder string, handleImapEmail func(*imap.Message), flags uint32,
	) {
		mailbox, err := clt.Select(folder, true)
		if err != nil {
			log.Fatal(err)
		}

		emails := make(chan *imap.Message, 10)
		done := make(chan error, 1)
		seqSet := new(imap.SeqSet)
		if flags&FecthLast10Emails != 0 {
			seqSet.AddRange(max(0, mailbox.Messages-10), mailbox.Messages)
			go func() {
				done <- clt.Fetch(seqSet, []imap.FetchItem{
					imap.FetchEnvelope, imap.FetchUid,
				}, emails)
			}()
		} else if flags&FetchBodyViaUID != 0 {
			seqSet.AddNum(uid)
			go func() {
				section := &imap.BodySectionName{
					Peek: false,
				}

				done <- clt.UidFetch(seqSet, []imap.FetchItem{
					imap.FetchUid, section.FetchItem(),
				}, emails)
			}()
		}

	fetchLoop:
		for {
			select {
			case imapEmail, ok := <-emails:
				if !ok {
					break fetchLoop
				}

				handleImapEmail(imapEmail)

			case err = <-done:
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}

	// joined client commands
	for {
		select {
		case folder := <-chCheckForNewMessages:
			fetchEmail(0, folder, appendImapEmailToUI, FecthLast10Emails)

		case emailUid := <-chDownloadEmailByUid:
			folder, _ := g_ui.foldersPane.GetItemText(
				g_ui.foldersPane.GetCurrentItem(),
			)
			fetchEmail(
				emailUid,
				folder,
				updateEmailBody,
				FetchBodyViaUID,
			)

		case <-chDownloadFolders:
			mailboxes := make(chan *imap.MailboxInfo, 10)
			done := make(chan error, 1)
			go func() {
				done <- clt.List("" /* base */, "*", mailboxes)
			}()

			for mailbox := range mailboxes {
				g_ui.app.QueueUpdateDraw(func() {
					g_ui.foldersPane.AddItem(mailbox.Name, "", 0,
						func() {
							g_ui.emailsPane.Clear()
							grabLatestEmails(mailbox.Name)
						})
				})
			}

			err := <-done
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

func updateEmailBody(imapEmail *imap.Message) {
	Require(imapEmail.Uid != 0, "requires uid")
	section := &imap.BodySectionName{
		Peek: false,
	}
	reader := imapEmail.GetBody(section)
	if reader == nil {
		updateStatusBar(
			fmt.Sprintf("email message: %d, has no body", imapEmail.Uid),
		)
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, reader)
	if err != nil {
		log.Fatal(err)
	}

	humanReadableSize := func(bytes int64) string {
		units := []string{"b", "kb", "mb", "gb", "tb", "pb"}
		size, unit := float64(bytes), 0
		for size >= 1024 && unit < len(units)-1 {
			size, unit = size/1024, unit+1
		}
		return fmt.Sprintf("%.1f %s", size, units[unit])
	}

	updateStatusBar(
		fmt.Sprintf(
			"downloaded email message: %d, size of %s",
			imapEmail.Uid,
			humanReadableSize(n),
		),
	)
	mailReader, err := mail.CreateReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		log.Fatal(err)
	}

	for {
		part, err := mailReader.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}

		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := header.ContentType()

			if strings.Contains(contentType, "text/plain") {
				data, _ := io.ReadAll(part.Body)
				PreviewPaneSetBody(imapEmail.Uid, string(data))
				return
			}
		}
	}
	g_ui.app.QueueUpdateDraw(func() {
		g_ui.previewPane.SetText(
			fmt.Sprintf(
				"no plaintext found, size was: %s",
				humanReadableSize(n),
			),
			false,
		)
	})
}

func appendImapEmailToUI(imapEmail *imap.Message) {
	email := Email{
		imapEmail.Uid,
		imapEmail.Envelope.Subject,
		imapEmail.Envelope.Date,
		imapEmail.Envelope.To[0].Address(),
		imapEmail.Envelope.From[0].Address(),
		imapEmail.Envelope.From[0].PersonalName,
		"",
	}

	g_emailsMtx.Lock()
	g_emailsTbl[email.id] = email
	g_emailsMtx.Unlock()

	g_ui.app.QueueUpdateDraw(func() {
		g_ui.emailsPane.InsertItem(
			0,
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
