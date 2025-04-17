package main

import (
	"fmt"
	"sort"
	"sync"
	"time"
	"unsafe"
)

type Email struct {
	uid         uint32
	seqNum      uint32
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

func cachedEmailFromUidsBinarySearch(emailsUidList []uint32, email Email) int {
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()

	folder := email.folder
	assertValidFolderName(folder)
	return sort.Search(len(g_ui.emailsUidList), func(k int) bool {
		e := g_emailFromUid[folder][emailsUidList[k]]
		return !emailCompare(*e, email)
	})
}

func emailCompare(e1 Email, e2 Email) bool {
	if e1.date == e2.date {
		return e1.uid > e2.uid
	}
	return e1.date.After(e2.date)
}

func cachedEmailByFolderBinarySearchLocked(email Email) int {
	Assert(g_emailsMu.TryLock() == false, "g_emailsMu needs to be locked")
	Require(email.uid != 0, "email.id required")
	Require(email.folder != "", "email.folder required")

	folder := email.folder
	assertValidFolderName(folder)
	return sort.Search(len(g_emailsFromFolder[folder]), func(k int) bool {
		return !emailCompare(*g_emailsFromFolder[folder][k], email)
	})
}

func cachedEmailByFolderInsertLocked(email *Email) (int, bool) {
	Assert(g_emailsMu.TryLock() == false, "g_emailsMu needs to be locked")
	Require(email.uid != 0, "email.id required")
	Require(email.folder != "", "email.folder required")
	folder := email.folder
	assertValidFolderName(folder)
	i := cachedEmailByFolderBinarySearchLocked(*email)
	if i < len(g_emailsFromFolder[folder]) &&
		g_emailsFromFolder[folder][i].uid == email.uid {
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

func cachedEmailRemoveViaSeqNum(folder string, seqNum uint32) int {
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()

	uidToRemove := uint32(0)
	iToRemove := -1
	for i, email := range g_emailsFromFolder[folder] {
		if email.seqNum == seqNum {
			Assert(iToRemove == -1, "found seqNum twice?")
			iToRemove = i
			uidToRemove = email.uid
		}
	}

	for _, email := range g_emailsFromFolder[folder] {
		if email.seqNum > seqNum && email.uid != uidToRemove {
			email.seqNum--
		}
	}

	g_emailsFromFolder[folder] = append(
		g_emailsFromFolder[folder][:iToRemove],
		g_emailsFromFolder[folder][iToRemove+1:]...)

	if iToRemove != -1 {
		n := len(g_emailFromUid[folder])
		delete(g_emailFromUid[folder], uidToRemove)
		Assert(n > len(g_emailFromUid[folder]), "emailFromUid not removed")
	}

	return iToRemove
}

func cachedEmailBodyUpdate(folder string, uid uint32, body string, size int64) {
	Require(body != "", "we need some body to update")
	assertValidFolderName(folder)
	g_emailsMu.Lock()
	email, ok := g_emailFromUid[folder][uid]
	Assert(ok, "we needed a valid envelope first before setting body")
	email.body = body
	email.size = uint64(size)
	assertEmailCorrectlyInCacheLocked(folder, email)
	g_emailsMu.Unlock()
}

func cachedEmailFromUid(folder string, uid uint32) Email {
	assertValidFolderName(folder)
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()
	return *g_emailFromUid[folder][uid]
}

func cachedEmailFromUidChecked(folder string, uid uint32) (Email, bool) {
	assertValidFolderName(folder)
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()
	email, exists := g_emailFromUid[folder][uid]
	if exists {
		return *email, exists
	} else {
		return Email{}, exists
	}
}

func cachedEmailEnvelopeSet(email *Email) {
	Require(email.uid != 0, "email.id required")
	Require(email.folder != "", "email.folder required")
	folder := email.folder
	assertValidFolderName(folder)
	g_emailsMu.Lock()
	emailsByFolder, ok := g_emailFromUid[folder]
	if !ok {
		emailsByFolder = make(map[uint32]*Email)
		g_emailFromUid[folder] = emailsByFolder
	}
	_, ok = emailsByFolder[email.uid]
	if !ok {
		emailsByFolder[email.uid] = email
	}
	cachedEmailByFolderInsertLocked(email)
	assertEmailCorrectlyInCacheLocked(folder, email)
	g_emailsMu.Unlock()
}

func cachedEmailFromFolderItemCount(folder string) int {
	assertValidFolderName(folder)
	g_emailsMu.Lock()
	defer g_emailsMu.Unlock()
	return len(g_emailsFromFolder[folder])
}

func assertEmailCorrectlyInCacheLocked(folder string, email *Email) {
	assertValidFolderName(folder)
	uid := email.uid
	i := cachedEmailByFolderBinarySearchLocked(*email)
	e1 := g_emailsFromFolder[folder][i]
	e2 := g_emailFromUid[folder][uid]
	Assert(
		unsafe.Pointer(e1) == unsafe.Pointer(e2),
		"caches are pointing to different emails",
	)
	Assert(e1.uid == uid, fmt.Sprintf("email uid: %d, not in cache", uid))
	Assert(e2.uid == uid, fmt.Sprintf("email uid: %d, not in cache", uid))
}
