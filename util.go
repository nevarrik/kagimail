package main

import (
	"runtime"
	"strings"
)

func Assert(condition bool, msg string) {
	if !condition {
		panic(msg)
	}
}

func AssertNotReachable(msg string) {
	panic(msg)
}

func Require(condition bool, msg string) {
	if !condition {
		panic(msg)
	}
}

func IsOnUiThread() bool {
	buf := make([]byte, 64)
	runtime.Stack(buf, false)
	return strings.HasPrefix(string(buf), "goroutine 1")
}
