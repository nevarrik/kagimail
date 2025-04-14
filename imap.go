package main

import (
	"bytes"
	"errors"
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

type FetchFolderRequest struct {
	folder string
	done   chan error
}

type FetchEmailBodyRequest struct {
	folder string
	uid    uint32
	done   chan error
}

type FetchFolderListRequest struct {
	done chan error
}

var (
	chFetchFolder     chan FetchFolderRequest
	chFetchEmailBody  chan FetchEmailBodyRequest
	chFetchFolderList chan FetchFolderListRequest
)

func fetchEmailBody(folder string, uid uint32) {
	done := make(chan error, 1)
	chFetchEmailBody <- FetchEmailBodyRequest{folder, uid, done}
	err := <-done
	if err != nil {
		updateStatusBar(fmt.Sprintf(
			"Unable to download email body for id %d: %v", uid, err))
	}
}

func fetchFolder(folder string) {
	done := make(chan error, 1)
	chFetchFolder <- FetchFolderRequest{folder, done}

	err := <-done
	if err != nil {
		updateStatusBar(fmt.Sprintf(
			"Unable to download messages for folder \"%s\": %v", folder, err))
	}
}

func fetchFolderList() {
	done := make(chan error, 1)
	chFetchFolderList <- FetchFolderListRequest{done}
	err := <-done
	if err != nil {
		updateStatusBar(
			fmt.Sprintf("Unable to download folder list: %v", err))
	}
}

func imapInit() {
	chFetchFolder = make(chan FetchFolderRequest, 10)
	chFetchEmailBody = make(chan FetchEmailBodyRequest, 10)
	chFetchFolderList = make(chan FetchFolderListRequest, 1)

	go imapWorker()

	go func() {
		fetchFolderList()
		if g_ui.foldersList.GetItemCount() > 0 {
			folder, _ := g_ui.foldersList.GetItemText(0)
			fetchFolder(folder)
		}
	}()
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
		handleImapEmail func(*imap.Message) error,
	) error {
	fetchLoop:
		// read from channel and use handleImapEmail handler
		for {
			select {
			case imapEmail, ok := <-chEmails:
				if !ok {
					break fetchLoop
				}

				err := handleImapEmail(imapEmail)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}

	fetchEmail := func(
		uid uint32,
		folder string,
		handleImapEmail func(*imap.Message) error,
		chAllDone chan error,
		flags uint32,
	) {
		mailbox, err := clt.Select(folder, true)
		if err != nil {
			chAllDone <- err
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

				seqSet := new(imap.SeqSet)
				seqSet.AddRange(uint32(lo+1), uint32(hi))
				go func() {
					chEmailsDone <- clt.Fetch(seqSet, []imap.FetchItem{
						imap.FetchEnvelope, imap.FetchUid,
					}, chEmails)
				}()

				err := handleEmails(chEmails, handleImapEmail)
				if err != nil {
					chAllDone <- err
					break
				}

				err = <-chEmailsDone
				if err != nil {
					chAllDone <- err
					break
				}
			}

			chAllDone <- nil
		}

		const (
			fetchUsingUid = 1 << iota
			fetchUsingSeq
		)

		fetchSingleEmail := func(
			uid uint32, items []imap.FetchItem, flags int,
		) {
			chEmails := make(chan *imap.Message, 10)
			seqSet := new(imap.SeqSet)
			seqSet.AddNum(uid)
			go func() {
				done := make(chan error, 1)
				if flags&fetchUsingUid != 0 {
					done <- clt.UidFetch(seqSet, items, chEmails)
				} else if flags&fetchUsingSeq != 0 {
					done <- clt.Fetch(seqSet, items, chEmails)
				} else {
					AssertNotReachable("needed fetchUsing flag")
				}

				err := <-done
				if err != nil {
					chAllDone <- err
				}
			}()

			chAllDone <- handleEmails(chEmails, handleImapEmail)
		}

		emailCountAfter := int(mailbox.Messages)
		if flags&fetchAllEmailsInFolder != 0 {
			notifyFetchAllStarted(folder, emailCountAfter)
			fetchMultipleEmails(0, emailCountAfter)
		} else if flags&fetchLatestEmails != 0 {
			g_emailsMtx.Lock()
			emailCountBefore := len(g_emailsFromFolder[folder])
			g_emailsMtx.Unlock()

			if emailCountBefore < emailCountAfter {
				notifyFetchLatestStarted(folder, emailCountAfter)
				fetchMultipleEmails(emailCountBefore, emailCountAfter)
			}
		} else if flags&fetchSingleEmailViaSeq != 0 {
			fetchSingleEmail(
				uid /* treat as seq */, []imap.FetchItem{
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
		case req := <-chFetchFolder:
			fetchEmail(
				0,
				req.folder,
				appendImapEmailToUI,
				req.done,
				fetchAllEmailsInFolder,
			)

		case req := <-chFetchEmailBody:
			fetchEmail(
				req.uid,
				req.folder,
				updateEmailBody,
				req.done,
				fetchEmailBodyViaUID,
			)

		case req := <-chFetchFolderList:
			mailboxes := make(chan *imap.MailboxInfo, 10)
			go func() {
				req.done <- clt.List(
					"" /* base folder hierarchy */, "*", mailboxes)
			}()

			for mailbox := range mailboxes {
				insertFolderToList(mailbox.Name)
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
				done := make(chan error)
				fetchEmail(
					0, folder, appendImapEmailToUI, done, fetchLatestEmails)
				err := <-done
				if err != nil {
					updateStatusBar(fmt.Sprintf(
						"Unable update mailbox \"%s\": %v", folder, err))
				}

			case *client.MessageUpdate:
				messageUpdate := update.(*client.MessageUpdate)
				folder := g_ui.emailsFolderSelected
				seqNum := messageUpdate.Message.SeqNum
				done := make(chan error)
				fetchEmail(
					seqNum, folder, appendImapEmailToUI, done,
					fetchSingleEmailViaSeq)
				err := <-done
				if err != nil {
					updateStatusBar(fmt.Sprintf(
						"Unable update message of seq \"%d\": %v", seqNum, err))
				}
			}
		}
	}
}

func updateEmailBody(imapEmail *imap.Message) error {
	Require(imapEmail.Uid != 0, "requires uid")
	section := &imap.BodySectionName{
		Peek: false,
	}
	reader := imapEmail.GetBody(section)
	if reader == nil {
		return errors.New("email message has no body")
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, reader)
	if err != nil {
		return errors.New(fmt.Sprintf("unable to copy email buffer: %v", err))
	}

	humanReadableSize := func(bytes int64) string {
		units := []string{"b", "kb", "mb", "gb", "tb", "pb"}
		size, unit := float64(bytes), 0
		for size >= 1024 && unit < len(units)-1 {
			size, unit = size/1024, unit+1
		}
		return fmt.Sprintf("%.1f %s", size, units[unit])
	}

	mailReader, err := mail.CreateReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return errors.New(fmt.Sprintf("unable to create reader: %v", err))
	}

	plainText := ""
	for {
		part, err := mailReader.NextPart()
		if err == io.EOF {
			break
		} else if err != nil {
			return errors.New(fmt.Sprintf(
				"unable to read all parts of emails: %v", err))
		}

		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := header.ContentType()

			if strings.Contains(contentType, "text/plain") {
				data, _ := io.ReadAll(part.Body)
				plainText = string(data)
				break
			}
		}
	}

	previewPaneSetBody(imapEmail.Uid, plainText)
	updateStatusBar(
		fmt.Sprintf("downloaded email message: %d, size of %s",
			imapEmail.Uid, humanReadableSize(n)))

	if plainText == "" {
		g_ui.app.QueueUpdateDraw(func() {
			g_ui.previewText.SetText(fmt.Sprintf(
				"no plaintext found, size was: %s",
				humanReadableSize(n),
			), false)
		})
	}

	return nil
}

func appendImapEmailToUI(imapEmail *imap.Message) error {
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
	return nil
}
