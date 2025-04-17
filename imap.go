package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/emersion/go-imap"
	sortthread "github.com/emersion/go-imap-sortthread"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

type FetchFolderRequest struct {
	folder    string
	mailCount chan int
	done      chan error
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
	emailCount := make(chan int, 1)
	done := make(chan error, 1)
	chFetchFolder <- FetchFolderRequest{folder, emailCount, done}

	n := <-emailCount
	notifyFetchAllStarted(folder, n)

	err := <-done
	notifyFetchAllFinished(err, folder)
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
	fetchSingle
)

func imapFetchViaCriteria(
	clt *client.Client,
	folder string,
	searchCriteria *imap.SearchCriteria,
	chAllDone chan error,
	flags uint,
) {
	collectEmails := func(folder string, chEmails chan *imap.Message) []*Email {
		var emails []*Email
		for {
			select {
			case imapEmail, ok := <-chEmails:
				if !ok {
					return emails
				}

				email := emailFromImapEmail(folder, imapEmail)
				emails = append(emails, email)
			}
		}
	}

	_, err := clt.Select(folder, true)
	if err != nil {
		chAllDone <- err
		return
	}
	sortClt := sortthread.NewSortClient(clt)

	sortCriteria := []sortthread.SortCriterion{
		{Field: sortthread.SortDate, Reverse: true},
	}

	// get a list of uids in sorted order
	//
	var uids []uint32
	if searchCriteria.SeqNum != nil {
		Assert(len(searchCriteria.SeqNum.Set) == 1, "only works with 1 seq")
		loWater := int(searchCriteria.SeqNum.Set[0].Start)
		hiWater := int(searchCriteria.SeqNum.Set[0].Stop) + 1

		searchChunkSize := 100
		for lo := hiWater; lo > loWater; {
			hi := lo
			lo -= searchChunkSize
			lo = max(loWater, lo)
			if flags&(fetchAllEmailsInFolder) != 0 {
				searchCriteria.SeqNum = new(imap.SeqSet)
				searchCriteria.SeqNum.AddRange(uint32(lo+1), uint32(hi))
			}

			uids_, err := sortClt.UidSort(sortCriteria, searchCriteria)
			if err != nil {
				chAllDone <- err
				return
			}

			uids = append(uids, uids_...)
		}
	} else if searchCriteria.Uid != nil {
		Assert(flags&fetchEmailBodyViaUID != 0,
			"this is the only case I know of where we hit this path")
		Assert(len(searchCriteria.Uid.Set) == 1, "only works with 1 uid")
		Assert(searchCriteria.Uid.Set[0].Start ==
			searchCriteria.Uid.Set[0].Stop, "only works with 1 uid")
		uids = append(uids, searchCriteria.Uid.Set[0].Start)
	}

	// fetch emails from list of uids
	//
	fetchChunkSize := 20
	seqSet := new(imap.SeqSet)
	n := 0
	for k, uid := range uids {
		if flags&fetchEmailBodyViaUID == 0 {
			email, exists := cachedEmailFromUidChecked(folder, uid)
			if exists {
				insertImapEmailToList(email)
				continue
			}
		}

		seqSet.AddNum(uid)
		n += 1
		if n == fetchChunkSize || (k == len(uids)-1 && n > 0) {
			chEmails := make(chan *imap.Message, fetchChunkSize)
			chFetchDone := make(chan error, 1)
			go func() {
				fi := []imap.FetchItem{imap.FetchUid}
				if flags&fetchEmailBodyViaUID != 0 {
					section := &imap.BodySectionName{
						Peek: false,
					}
					fi = append(fi, section.FetchItem())
				} else {
					fi = append(fi, imap.FetchEnvelope)
				}
				chFetchDone <- clt.UidFetch(seqSet, fi, chEmails)
			}()

			if flags&fetchEmailBodyViaUID != 0 {
			fetchLoop:
				for {
					select {
					case imapEmail, ok := <-chEmails:
						if !ok {
							break fetchLoop
						}
						updateEmailBody(folder, imapEmail)
					}
				}
				err = <-chFetchDone
				if err != nil {
					chAllDone <- err
					return
				}
			} else {
				emails := collectEmails(folder, chEmails)
				sort.Slice(emails, func(i, j int) bool {
					return emailCompare(*emails[i], *emails[j])
				})
				for _, email := range emails {
					cachedEmailEnvelopeSet(email)
					insertImapEmailToList(*email)
				}

				err = <-chFetchDone
				if err != nil {
					chAllDone <- err
					return
				}
			}
		}
	}

	chAllDone <- nil
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
				mailbox, err := cltFillLists.Select(req.folder, true)
				if err != nil {
					req.done <- err
					return
				}
				req.mailCount <- int(mailbox.Messages)

				criteria := imap.NewSearchCriteria()
				criteria.SeqNum = new(imap.SeqSet)
				criteria.SeqNum.AddRange(1, mailbox.Messages)
				imapFetchViaCriteria(
					cltFillLists,
					req.folder,
					criteria,
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
