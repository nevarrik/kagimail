package main

import (
	"fmt"
	"sort"
	"time"
)

type Email struct {
	id          uint32
	subject     string
	date        time.Time
	toAddress   string
	fromAddress string
	fromName    string
	body        string
	size        uint64
}

func cachedEmailByFolderBinarySearch(folder string, email Email) int {
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()
	return cachedEmailByFolderBinarySearchLocked(folder, email)
}

func cachedEmailByFolderBinarySearchLocked(folder string, email Email) int {
	Assert(g_emailsMu.TryLock() == false, "g_emailsMu needs to be locked")

	fnDateCompare := func(e1 Email, e2 Email) bool {
		if e1.date == e2.date {
			return e1.id > e2.id
		}
		return e1.date.After(e2.date)
	}

	return sort.Search(len(g_emailsFromFolder[folder]), func(k int) bool {
		return !fnDateCompare(*g_emailsFromFolder[folder][k], email)
	})
}

func cachedEmailByFolderInsertLocked(folder string, email *Email) (int, bool) {
	Assert(g_emailsMu.TryLock() == false, "g_emailsMu needs to be locked")
	i := cachedEmailByFolderBinarySearchLocked(folder, *email)
	if i < len(g_emailsFromFolder[folder]) &&
		g_emailsFromFolder[folder][i].id == email.id {
		return i, true
	}

	g_emailsFromFolder[folder] = append(
		g_emailsFromFolder[folder],
		&Email{},
	)
	copy(
		g_emailsFromFolder[folder][i+1:],
		g_emailsFromFolder[folder][i:],
	)
	g_emailsFromFolder[folder][i] = email
	return i, false
}

func cachedEmailBodyUpdate(folder string, uid uint32, body string, size int64) {
	Require(body != "", "we need some body to update")
	g_emailsMu.Lock()
	email, ok := g_emailFromUid[folder][uid]
	Assert(ok, "we needed a valid envelope first before setting body")
	email.body = body
	email.size = uint64(size)
	i := cachedEmailByFolderBinarySearchLocked(folder, *email)

	e1 := g_emailsFromFolder[folder][i]
	e2 := g_emailFromUid[folder][uid]
	Assert(e1.id == uid, fmt.Sprintf("email uid: %d, not in cache", uid))
	Assert(e2.id == uid, fmt.Sprintf("email uid: %d, not in cache", uid))
	Assert(e1.body != "" && e2.body != "", "body not updated correctly")
	Assert(e1.body == e2.body, "body not updated correctly across maps")
	g_emailsMu.Unlock()
}

func cachedEmailFromUid(folder string, uid uint32) Email {
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()
	return *g_emailFromUid[folder][uid]
}

func cachedEmailEnvelopeSet(folder string, email *Email) {
	Require(email.id != 0, "email.id required")
	g_emailsMu.Lock()
	emailsByFolder, ok := g_emailFromUid[folder]
	if !ok {
		emailsByFolder = make(map[uint32]*Email)
		g_emailFromUid[folder] = emailsByFolder
	}
	emailsByFolder[email.id] = email
	cachedEmailByFolderInsertLocked(folder, email)
	g_emailsMu.Unlock()
}
