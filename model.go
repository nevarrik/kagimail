package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type Email struct {
	id          uint32
	folder      string
	subject     string
	date        time.Time
	toAddress   string
	fromAddress string
	fromName    string
	body        string
	size        uint64
}

var (
	g_emailsMu         sync.Mutex
	g_emailFromUid     map[string]map[uint32]*Email
	g_emailsFromFolder map[string][]*Email
)

func modelInit() {
	g_emailFromUid = make(map[string]map[uint32]*Email)
	g_emailsFromFolder = make(map[string][]*Email)
}

func cachedEmailByFolderBinarySearch(email Email) int {
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()
	return cachedEmailByFolderBinarySearchLocked(email)
}

func cachedEmailByFolderBinarySearchLocked(email Email) int {
	Assert(g_emailsMu.TryLock() == false, "g_emailsMu needs to be locked")
	Require(email.id != 0, "email.id required")
	Require(email.folder != "", "email.folder required")

	fnDateCompare := func(e1 Email, e2 Email) bool {
		if e1.date == e2.date {
			return e1.id > e2.id
		}
		return e1.date.After(e2.date)
	}

	folder := email.folder
	return sort.Search(len(g_emailsFromFolder[folder]), func(k int) bool {
		return !fnDateCompare(*g_emailsFromFolder[folder][k], email)
	})
}

func cachedEmailByFolderInsertLocked(email *Email) (int, bool) {
	Assert(g_emailsMu.TryLock() == false, "g_emailsMu needs to be locked")
	Require(email.id != 0, "email.id required")
	Require(email.folder != "", "email.folder required")
	folder := email.folder
	i := cachedEmailByFolderBinarySearchLocked(*email)
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
	i := cachedEmailByFolderBinarySearchLocked(*email)

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

func cachedEmailEnvelopeSet(email *Email) {
	Require(email.id != 0, "email.id required")
	Require(email.folder != "", "email.folder required")
	folder := email.folder
	g_emailsMu.Lock()
	emailsByFolder, ok := g_emailFromUid[folder]
	if !ok {
		emailsByFolder = make(map[uint32]*Email)
		g_emailFromUid[folder] = emailsByFolder
	}
	emailsByFolder[email.id] = email
	cachedEmailByFolderInsertLocked(email)
	g_emailsMu.Unlock()
}

func cachedEmailFromFolderItemCount(folder string) int {
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()
	return len(g_emailsFromFolder[folder])
}
