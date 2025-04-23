package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/cloudfoundry/jibber_jabber"
	"github.com/goodsign/monday"
)

func Assert(condition bool, msg string) {
	if !condition {
		log.Print("Assert failed: ", msg)
		panic(msg)
	}
}

func Assume(condition bool, msg string) {
	if !condition {
		log.Print("Assume failed: ", msg)
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

func FormatLocalizedTime(time time.Time) string {
	userLocale, err := jibber_jabber.DetectLanguage()
	if err != nil {
		userLocale = "en_US"
	}
	locale := monday.Locale(userLocale)
	longDateFormat, ok := monday.FullFormatsByLocale[locale]
	if !ok {
		longDateFormat = monday.DefaultFormatEnUSFull
	}
	longTimeFormat, ok := monday.TimeFormatsByLocale[locale]
	if !ok {
		longTimeFormat = monday.DefaultFormatEnUSTime
	}

	return monday.Format(
		time, longDateFormat+" "+longTimeFormat, locale)
}

func FormatAsRelativeTimeIfWithin24Hours(ts time.Time) string {
	du := time.Since(ts)
	if du < time.Minute {
		return "~now"
	} else if du < time.Hour {
		return fmt.Sprintf("~%dm", int(du.Minutes()))
	} else if du < time.Hour*24 {
		return fmt.Sprintf("~%dh", int(du.Hours()))
	}
	return ts.Format("2 Jan")
}
