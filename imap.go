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
	assertValidFolderName(folder)
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
	assertValidFolderName(folder)
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
		Assert(loWater >= 1, "0 isn't the first mail in imap, use 1")

		searchChunkSize := 100
		for hi := hiWater; hi > loWater; hi -= searchChunkSize {
			lo := hi - searchChunkSize
			lo = max(loWater, lo+1)
			if flags&fetchAllEmailsInFolder != 0 {
				searchCriteria.SeqNum = new(imap.SeqSet)
				searchCriteria.SeqNum.AddRange(uint32(lo), uint32(hi))
			}

			uids_, err := sortClt.UidSort(sortCriteria, searchCriteria)
			if err != nil {
				chAllDone <- err
				return
			}

			uids = append(uids, uids_...)
		}

		// perf: ensure no overlapping ranges when chunking uids
		seen := make(map[uint32]bool)
		for _, x := range uids {
			_, ok := seen[x]
			Assert(!ok, "duplicate uid")
			seen[x] = true
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
				fi := []imap.FetchItem{imap.FetchUid, imap.FetchFlags}
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

	//  high-priority view messages
	go func() {
		cltDownloadBody := imapLogin()
		defer cltDownloadBody.Logout()

		var reqLast FetchEmailBodyRequest
		for {
			select {
			case req := <-chFetchEmailBody: // drain channel, use latest
				reqLast = req

			default:
				if reqLast.uid == 0 {
					continue
				}

				criteria := imap.NewSearchCriteria()
				criteria.Uid = new(imap.SeqSet)
				criteria.Uid.AddNum(reqLast.uid)
				imapFetchViaCriteria(
					cltDownloadBody,
					reqLast.folder,
					criteria,
					reqLast.done,
					fetchEmailBodyViaUID,
				)
				reqLast = FetchEmailBodyRequest{}
			}
		}
	}()

	// downloading folders and all emails in a folder
	go func() {
		cltFillLists := imapLogin()
		defer cltFillLists.Logout()
		for {
			select {
			case req := <-chFetchFolderList:
				mailboxes := make(chan *imap.MailboxInfo, 10)
				go func() {
					req.done <- cltFillLists.List(
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
	//
	chImapUpdates := make(chan client.Update, 10)

	// separate client to listen for idle commands to fetch new incoming emails
	go func() {
		cltIdle := imapLogin()
		defer cltIdle.Logout()
		_, err = cltIdle.Select("Inbox", true)
		if err != nil {
			log.Fatal(err)
		}

		chImapUpdatesStop := make(chan struct{}, 1)
		chImapUpdatesDone := make(chan error, 1)
		cltIdle.Updates = chImapUpdates
		chImapUpdatesDone <- cltIdle.Idle(chImapUpdatesStop, nil)
	}()

	// separate client to download emails in response to changes
	go func() {
		cltUpdates := imapLogin()
		defer cltUpdates.Logout()

		for {
			select {
			case update := <-chImapUpdates:
				switch update.(type) {
				case *client.MailboxUpdate:
					mailboxUpdate := update.(*client.MailboxUpdate)
					folder := getNormalizedImapFolderName(
						mailboxUpdate.Mailbox.Name)
					emailsInStore := uint32(
						cachedEmailFromFolderItemCount(folder))
					emailsAvailable := mailboxUpdate.Mailbox.Messages
					if emailsAvailable <= emailsInStore {
						continue
					}

					mailbox, err := cltUpdates.Select(folder, true)
					if err != nil {
						updateStatusBar(fmt.Sprintf(
							"Unable update mailbox \"%s\": %v", folder, err))
						continue
					}
					notifyFetchLatestStarted(folder, int(mailbox.Messages))

					criteria := imap.NewSearchCriteria()
					criteria.SeqNum = new(imap.SeqSet)
					criteria.SeqNum.AddRange(emailsInStore+1, emailsAvailable)
					done := make(chan error, 1)
					imapFetchViaCriteria(
						cltUpdates, folder, criteria, done, fetchLatestEmails)

					err = <-done
					Assert(cachedEmailFromFolderItemCount(folder) ==
						int(emailsAvailable),
						"email count not matching mailbox update count")

					if err != nil {
						updateStatusBar(fmt.Sprintf(
							"Unable update mailbox \"%s\": %v", folder, err))
						continue
					}

				case *client.MessageUpdate:
					messageUpdate := update.(*client.MessageUpdate)
					folder := g_ui.folderSelected
					seqNum := messageUpdate.Message.SeqNum
					done := make(chan error, 1)
					criteria := imap.NewSearchCriteria()
					criteria.SeqNum = new(imap.SeqSet)
					criteria.SeqNum.AddNum(seqNum)
					imapFetchViaCriteria(
						cltUpdates, folder, criteria, done, fetchSingle)
					err := <-done
					if err != nil {
						updateStatusBar(fmt.Sprintf(
							"Unable update message of seq \"%d\": %v", seqNum, err))
					}

				case *client.ExpungeUpdate:
					expungeUpdate := update.(*client.ExpungeUpdate)
					folder := "Inbox"
					k := cachedEmailRemoveViaSeqNum(
						folder, expungeUpdate.SeqNum)
					if k != -1 {
						removeEmailFromList(k)
					}
				} // end: switch update.(type)
			} // end: select
		} // end: for
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

	if plainText == "" {
		plainText = fmt.Sprintf(
			"<no plaintext message found, email size: %s>",
			FormatHumanReadableSize(n),
		)
	}

	cachedEmailBodyUpdate(folder, imapEmail.Uid, plainText, n)
	return nil
}

func emailFromImapEmail(folder string, imapEmail *imap.Message) *Email {
	var seenFlag bool
	for _, flag := range imapEmail.Flags {
		if flag == imap.SeenFlag {
			seenFlag = true
			break
		}
	}

	email := Email{
		uid:         imapEmail.Uid,
		seqNum:      imapEmail.SeqNum,
		folder:      getNormalizedImapFolderName(folder),
		subject:     imapEmail.Envelope.Subject,
		date:        imapEmail.Envelope.Date,
		toAddress:   "",
		fromAddress: "",
		fromName:    "",
		body:        "",
		size:        0,
		isRead:      seenFlag,
	}

	if len(imapEmail.Envelope.To) > 0 {
		email.toAddress = imapEmail.Envelope.To[0].Address()
	}

	if len(imapEmail.Envelope.From) > 0 {
		email.fromAddress = imapEmail.Envelope.From[0].Address()
		email.fromName = imapEmail.Envelope.From[0].PersonalName
	}

	return &email
}

func getNormalizedImapFolderName(folder string) string {
	if strings.ToLower(folder) == "inbox" {
		return "Inbox"
	}
	return folder
}

func assertValidFolderName(folder string) {
	Assert(strings.ToLower(folder) != "inbox" || folder == "Inbox",
		"folder inbox has inconsistent casing of: %s, should be \"Inbox\"")
}
