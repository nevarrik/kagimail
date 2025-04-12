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
	chFetchMessagesByFolder chan string
	chFetchEmailBodyByUid   chan uint32
	chFetchFolders          chan bool
)

func fetchEmailBody(id uint32) {
	chFetchEmailBodyByUid <- id
}

func fetchFolderEmails(folder string) {
	chFetchMessagesByFolder <- folder
}

func imapInit() {
	chFetchMessagesByFolder = make(chan string, 10)
	chFetchEmailBodyByUid = make(chan uint32, 10)
	chFetchFolders = make(chan bool, 1)

	go imapWorker()
	chFetchFolders <- true
	fetchFolderEmails("inbox")
}

func imapLogin() *client.Client {
	clt, err := client.DialTLS(g_config.IMAPHost+":993", nil)
	if err != nil {
		log.Fatal(err)
	}

	err = clt.Login(g_config.Email, g_config.Password)
	if err != nil {
		log.Fatal(err)
	}

	return clt
}

func imapWorker() {
	_, err := toml.DecodeFile("kagimail.toml", &g_config)
	if err != nil {
		log.Fatal(err)
	}

	clt := imapLogin()
	defer clt.Logout()
	chImapUpdates := make(chan client.Update, 10)

	// separate client to listen for idle commands to fetch new incoming emails
	go func() {
		cltIdle := imapLogin()
		defer cltIdle.Logout()
		_, err = cltIdle.Select("inbox", true)
		if err != nil {
			log.Fatal(err)
		}

		chImapUpdatesStop := make(chan struct{}, 1)
		chImapUpdatesDone := make(chan error, 1)
		cltIdle.Updates = chImapUpdates
		chImapUpdatesDone <- cltIdle.Idle(chImapUpdatesStop, nil)
	}()

	const (
		fetchEmailBodyViaUID = 1 << iota
		fetchAllEmailsInFolder
		fetchLatestEmails
		fetchSingleEmailViaSeq
	)

	handleEmails := func(
		chEmails chan *imap.Message,
		chEmailsDone chan error,
		handleImapEmail func(*imap.Message),
	) {
	fetchLoop:
		// read from channel and use handleImapEmail handler
		for {
			select {
			case imapEmail, ok := <-chEmails:
				if !ok {
					break fetchLoop
				}

				handleImapEmail(imapEmail)

			case err = <-chEmailsDone:
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}

	fetchEmail := func(
		uid uint32,
		folder string,
		handleImapEmail func(*imap.Message),
		flags uint32,
	) {
		mailbox, err := clt.Select(folder, true)
		if err != nil {
			log.Fatal(err)
		}

		fetchMultipleEmails := func(lowWater int, hiWater int) {
			chunkSize := 10
			// retrieving in chunks, from newest to oldest
			for lo := int(hiWater); lo > lowWater; {
				// construct request to fetch chEmails
				chEmails := make(chan *imap.Message, 10)
				chEmailsDone := make(chan error, 1)
				hi := lo
				lo -= chunkSize
				lo = max(0, lo)

				log.Printf("fetching emails: %d to %d", lo+1, hi)
				seqSet := new(imap.SeqSet)
				seqSet.AddRange(uint32(lo+1), uint32(hi))
				go func() {
					chEmailsDone <- clt.Fetch(seqSet, []imap.FetchItem{
						imap.FetchEnvelope, imap.FetchUid,
					}, chEmails)
				}()

				handleEmails(chEmails, chEmailsDone, handleImapEmail)
			}
		}

		const (
			fetchUsingUid = 1 << iota
			fetchUsingSeq
		)

		fetchSingleEmail := func(
			uid uint32, items []imap.FetchItem, flags int,
		) {
			chEmails := make(chan *imap.Message, 10)
			chEmailsDone := make(chan error, 1)
			seqSet := new(imap.SeqSet)
			seqSet.AddNum(uid)
			go func() {
				if flags&fetchUsingUid != 0 {
					chEmailsDone <- clt.UidFetch(seqSet, items, chEmails)
				} else if flags&fetchUsingSeq != 0 {
					chEmailsDone <- clt.Fetch(seqSet, items, chEmails)
				} else {
					AssertNotReachable("needed fetchUsing flag")
				}
			}()

			handleEmails(chEmails, chEmailsDone, handleImapEmail)
		}

		if flags&fetchAllEmailsInFolder != 0 {
			notifyFetchStarted(folder, int(mailbox.Messages))
			fetchMultipleEmails(0, int(mailbox.Messages))
		} else if flags&fetchLatestEmails != 0 {
			g_emailsMtx.Lock()
			mailCountBefore := len(g_emailsFromFolder[folder])
			g_emailsMtx.Unlock()

			fetchMultipleEmails(mailCountBefore, int(mailbox.Messages))
		} else if flags&fetchSingleEmailViaSeq != 0 {
			fetchSingleEmail(uid /* treat as seq */, []imap.FetchItem{
				imap.FetchEnvelope, imap.FetchUid,
			}, fetchUsingSeq)
		} else if flags&fetchEmailBodyViaUID != 0 {
			section := &imap.BodySectionName{
				Peek: false,
			}
			fetchSingleEmail(uid, []imap.FetchItem{
				imap.FetchUid, section.FetchItem(),
			}, fetchUsingUid)
		}
	} // end: fetchEmail := func( ...

	// joined imap client commands
	for {
		select {
		case folder := <-chFetchMessagesByFolder:
			fetchEmail(0, folder, appendImapEmailToUI, fetchAllEmailsInFolder)

		case emailUid := <-chFetchEmailBodyByUid:
			// fixme: is it wise to be querying the ui here?
			folder, _ := g_ui.foldersPane.GetItemText(
				g_ui.foldersPane.GetCurrentItem(),
			)
			fetchEmail(
				emailUid,
				folder,
				updateEmailBody,
				fetchEmailBodyViaUID,
			)

		case <-chFetchFolders:
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

		case update := <-chImapUpdates:
			switch update.(type) {
			case *client.MailboxUpdate:
				folder := g_ui.emailsFolderSelected
				g_emailsMtx.Lock()
				mailCountBefore := len(g_emailsFromFolder[folder])
				g_emailsMtx.Unlock()

				mailboxUpdate := update.(*client.MailboxUpdate)
				if int(mailboxUpdate.Mailbox.Messages) <= mailCountBefore {
					continue
				}
				fetchEmail(0, folder, appendImapEmailToUI, fetchLatestEmails)

			case *client.MessageUpdate:
				messageUpdate := update.(*client.MessageUpdate)
				folder := g_ui.emailsFolderSelected
				seqNum := messageUpdate.Message.SeqNum
				fetchEmail(
					seqNum, folder, appendImapEmailToUI, fetchSingleEmailViaSeq)
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
