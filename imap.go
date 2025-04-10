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
		FetchAllEmails
	)

	fetchEmail := func(
		uid uint32, folder string, handleImapEmail func(*imap.Message), flags uint32,
	) {
		mailbox, err := clt.Select(folder, true)
		if err != nil {
			log.Fatal(err)
		}
		notifyFetchStarted(folder, int(mailbox.Messages))

		k := 0
		n := 0
		chunkSize := 10
		// retrieving in chunks, from newest to oldest
		for lo := int(mailbox.Messages); lo > 0; {
			// construct request to fetch emails
			emails := make(chan *imap.Message, 10)
			done := make(chan error, 1)
			seqSet := new(imap.SeqSet)
			hi := lo
			lo -= chunkSize
			lo = max(0, lo)

			if flags&FetchAllEmails != 0 {
				seqSet.AddRange(uint32(lo+1), uint32(hi))
				n += hi - lo
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
			// read from channel and use handleImapEmail handler
			for {
				select {
				case imapEmail, ok := <-emails:
					if !ok {
						break fetchLoop
					}

					k++
					handleImapEmail(imapEmail)

				case err = <-done:
					if err != nil {
						log.Fatal(err)
					}
				}
			}
		} // end: for lo := ...
	} // end: fetchEmail := func( ...

	// joined client commands
	for {
		select {
		case folder := <-chCheckForNewMessages:
			fetchEmail(0, folder, appendImapEmailToUI, FetchAllEmails)

		case emailUid := <-chDownloadEmailByUid:
			// fixme: is it wise to be querying the ui here?
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
				insertFolderToList(mailbox.Name)
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
				previewPaneSetBody(imapEmail.Uid, string(data))
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
		"",
		"",
		"",
		"",
	}

	if len(imapEmail.Envelope.To) > 0 {
		email.toAddress = imapEmail.Envelope.To[0].Address()
	}

	if len(imapEmail.Envelope.From) > 0 {
		email.fromAddress = imapEmail.Envelope.From[0].Address()
		email.fromName = imapEmail.Envelope.From[0].PersonalName
	}

	g_emailsMtx.Lock()
	g_emailFromUid[email.id] = email
	g_emailsMtx.Unlock()

	insertImapEmailToList(email)
}
