package main

import (
	"fmt"
	"strings"

	"github.com/cloudfoundry/jibber_jabber"
	"github.com/gdamore/tcell/v2"
	"github.com/goodsign/monday"
)

const (
	PreviewMode = 1 << iota
	QuickReplyMode
	ReplyMode
)

func KeyHandler(event *tcell.EventKey) *tcell.EventKey {
	pane := g_ui.app.GetFocus()
	mode := 0
	if g_ui.previewPane.GetTitle() == "Preview" {
		mode = PreviewMode
	} else if g_ui.previewPane.GetTitle() == "Quick Reply" {
		mode = QuickReplyMode
	}

	inEmailsOrPreview := pane == g_ui.emailsPane || pane == g_ui.previewPane
	if mode == PreviewMode && inEmailsOrPreview {
		if (event.Key() == tcell.KeyRune && event.Rune() == 'r') ||
			event.Key() == tcell.KeyCtrlR {
			g_ui.previewPane.SetTitle("Quick Reply")

			originalText := g_ui.previewPane.GetText()
			var reply strings.Builder

			g_emailsMtx.Lock()
			date := g_emailsTbl[g_ui.previewUid].date
			author := g_emailsTbl[g_ui.previewUid].fromName
			g_emailsMtx.Unlock()

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

			reply.WriteString(fmt.Sprintf("On %s %s wrote:\n",
				monday.Format(date, longDateFormat+" "+longTimeFormat, locale), author))

			for _, line := range strings.Split(originalText, "\n") {
				line = ">" + line
				reply.WriteString(line + "\n")
			}

			g_ui.previewPane.SetText(reply.String(), false)
			return nil
		}

		switch event.Key() {
		}
	}

	if mode == QuickReplyMode && inEmailsOrPreview {
		if event.Key() == tcell.KeyCtrlJ { // this is sent on ^enter
			g_emailsMtx.Lock()
			email := g_emailsTbl[g_ui.previewUid]
			g_emailsMtx.Unlock()

			email.body = g_ui.previewPane.GetText()
			email.toAddress = email.fromAddress
			email.fromAddress = g_config.Email
			email.fromName = g_config.DisplayName
			subject := strings.TrimSpace(email.subject)
			if !strings.HasPrefix(strings.ToLower(subject), "re:") {
				subject = "Re: " + subject
			}
			email.subject = subject

			sendEmail(email)
			return nil
		}
	}

	switch event.Key() {
	case tcell.KeyEscape:
		fallthrough
	case tcell.KeyCtrlC:
		fallthrough
	case tcell.KeyCtrlQ:
		g_ui.app.Stop()
		return nil

	case tcell.KeyTab:
		if pane == g_ui.foldersPane {
			g_ui.app.SetFocus(g_ui.emailsPane)
		} else if pane == g_ui.emailsPane {
			g_ui.app.SetFocus(g_ui.previewPane)
		} else if pane == g_ui.previewPane {
			g_ui.app.SetFocus(g_ui.foldersPane)
		} else {
			AssertNotReachable("coming from a control we don't know about")
		}
		return nil

	case tcell.KeyBacktab:
		if pane == g_ui.foldersPane {
			g_ui.app.SetFocus(g_ui.previewPane)
		} else if pane == g_ui.emailsPane {
			g_ui.app.SetFocus(g_ui.foldersPane)
		} else if pane == g_ui.previewPane {
			g_ui.app.SetFocus(g_ui.emailsPane)
		} else {
			AssertNotReachable("coming from a control we don't know about")
		}
		return nil
	}

	updateStatusBar("key: " + event.Name())
	return event
}
