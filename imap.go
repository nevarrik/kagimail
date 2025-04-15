package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

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
	var err error = nil
	var flags uint = 0
	if cachedEmailFromUid(folder, uid).body == "" {
		notifyFetchEmailBodyStarted(folder, uid)
		done := make(chan error, 1)
		chFetchEmailBody <- FetchEmailBodyRequest{folder, uid, done}
		err = <-done
	} else {
		flags |= notifyFetchEmailPulledFromCache
	}
	notifyFetchEmailBodyFinished(err, folder, uid, flags)
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

const (
	fetchEmailBodyViaUID = 1 << iota
	fetchAllEmailsInFolder
	fetchLatestEmails
	fetchSingleEmailViaSeq
)

func imapFetchEmails(clt *client.Client,
	uid uint32,
	folder string,
	handleImapEmail func(string, *imap.Message) error,
	chAllDone chan error,
	flags uint,
) {
	handleEmails := func(folder string, chEmails chan *imap.Message) error {
	fetchLoop:
		for {
			select {
			case imapEmail, ok := <-chEmails:
				if !ok {
					break fetchLoop
				}

				err := handleImapEmail(folder, imapEmail)
				if err != nil {
					return err
				}
			}
		}

		return nil
	}

	fetchMultipleEmails := func(lowWater int, hiWater int) {
		chunkSize := 10
		// retrieving in chunks, from newest to oldest
		for lo := int(hiWater); lo > lowWater; {
			// construct request to fetch emails
			chEmails := make(chan *imap.Message, 10)
			chFetchDone := make(chan error, 1)
			hi := lo
			lo -= chunkSize
			lo = max(0, lo)

			seqSet := new(imap.SeqSet)
			seqSet.AddRange(uint32(lo+1), uint32(hi))
			go func() {
				fi := []imap.FetchItem{
					imap.FetchEnvelope, imap.FetchUid,
				}
				chFetchDone <- clt.Fetch(seqSet, fi, chEmails)
			}()

			err := handleEmails(folder, chEmails)
			if err != nil {
				chAllDone <- err
				break
			}

			err = <-chFetchDone
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

		chAllDone <- handleEmails(folder, chEmails)
	}

	mailbox, err := clt.Select(folder, true)
	if err != nil {
		chAllDone <- err
	}
	emailCountAfter := int(mailbox.Messages)

	if flags&fetchAllEmailsInFolder != 0 {
		notifyFetchAllStarted(folder, emailCountAfter)
		fetchMultipleEmails(0, emailCountAfter)
	} else if flags&fetchLatestEmails != 0 {
		emailCountBefore := cachedEmailFromFolderItemCount(folder)
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
}

func imapWorker() {
	_, err := toml.DecodeFile("kagimail.toml", &g_config)
	if err != nil {
		log.Fatal(err)
	}

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

	//  high-priority view messages
	go func() {
		clt := imapLogin()
		defer clt.Logout()
		for {
			select {
			case req := <-chFetchEmailBody:
				imapFetchEmails(
					clt,
					req.uid,
					req.folder,
					updateEmailBody,
					req.done,
					fetchEmailBodyViaUID,
				)
			}
		}
	}()

	// downloading folders and all emails in a folder
	go func() {
		clt := imapLogin()
		defer clt.Logout()
		for {
			select {
			case req := <-chFetchFolderList:
				mailboxes := make(chan *imap.MailboxInfo, 10)
				go func() {
					req.done <- clt.List(
						"" /* base folder hierarchy */, "*", mailboxes)
				}()

				for mailbox := range mailboxes {
					insertFolderToList(mailbox.Name)
				}

			case req := <-chFetchFolder:
				imapFetchEmails(
					clt,
					0,
					req.folder,
					appendImapEmailToUI,
					req.done,
					fetchAllEmailsInFolder,
				)
			}
		}
	}()

	// imap idle handlers
	go func() {
		clt := imapLogin()
		defer clt.Logout()

		for {
			select {
			case update := <-chImapUpdates:
				switch update.(type) {
				case *client.MailboxUpdate:
					folder := g_ui.folderSelected
					mailCountBefore := cachedEmailFromFolderItemCount(folder)
					mailboxUpdate := update.(*client.MailboxUpdate)
					if int(mailboxUpdate.Mailbox.Messages) <= mailCountBefore {
						continue
					}
					done := make(chan error)
					imapFetchEmails(
						clt, 0, folder, appendImapEmailToUI, done, fetchLatestEmails)
					err := <-done
					if err != nil {
						updateStatusBar(fmt.Sprintf(
							"Unable update mailbox \"%s\": %v", folder, err))
					}

				case *client.MessageUpdate:
					messageUpdate := update.(*client.MessageUpdate)
					folder := g_ui.folderSelected
					seqNum := messageUpdate.Message.SeqNum
					done := make(chan error)
					imapFetchEmails(
						clt, seqNum, folder, appendImapEmailToUI, done,
						fetchSingleEmailViaSeq)
					err := <-done
					if err != nil {
						updateStatusBar(fmt.Sprintf(
							"Unable update message of seq \"%d\": %v", seqNum, err))
					}
				}
			}
		}
	}()
}

func updateEmailBody(folder string, imapEmail *imap.Message) error {
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

	cachedEmailBodyUpdate(folder, imapEmail.Uid, plainText, n)
	return nil
}

func appendImapEmailToUI(folder string, imapEmail *imap.Message) error {
	email := Email{
		imapEmail.Uid,
		folder,
		imapEmail.Envelope.Subject,
		imapEmail.Envelope.Date,
		"",
		"",
		"",
		"",
		0,
	}

	if len(imapEmail.Envelope.To) > 0 {
		email.toAddress = imapEmail.Envelope.To[0].Address()
	}

	if len(imapEmail.Envelope.From) > 0 {
		email.fromAddress = imapEmail.Envelope.From[0].Address()
		email.fromName = imapEmail.Envelope.From[0].PersonalName
	}

	cachedEmailEnvelopeSet(&email)
	insertImapEmailToList(email)
	return nil
}
