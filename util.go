package main

import (
	"fmt"
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
	return strings.HasPrefix(string(buf), "goroutine 1 ")
}

func FormatHumanReadableSize(bytes int64) string {
	units := []string{"b", "kb", "mb", "gb", "tb", "pb"}
	size, unit := float64(bytes), 0
	for size >= 1024 && unit < len(units)-1 {
		size, unit = size/1024, unit+1
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}
