package main

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/mail.v2"
)

var chSendMails chan Email

func smtpInit() {
	chSendMails = make(chan Email, 10)
	go smtpWorker()
}

func smtpWorker() {
	for {
		select {
		case email := <-chSendMails:
			msg := mail.NewMessage()
			msg.SetHeader(
				"From",
				msg.FormatAddress(email.fromAddress, email.fromName),
			)
			msg.SetHeader("To", email.toAddress)
			if email.ccAddress != "" {
				msg.SetHeader("Cc", email.ccAddress)
			}
			msg.SetHeader("Subject", email.subject)
			msg.SetBody("text/plain", email.body)

			dialer := mail.NewDialer(g_config.SMTPHost, 465,
				g_config.Email, g_config.Password)
			err := dialer.DialAndSend(msg)
			if err != nil {
				updateStatusBar(fmt.Sprintf("Couldn't send email: %v", err))
				break
			}

			formattedTime := time.Now().Format(time.Stamp)
			updateStatusBar(fmt.Sprintf(
				"Email sent to: %s at %s", email.toAddress, formattedTime))
		}
	}
}

func replyEmail(emailOriginal Email, body string) {
	Require(emailOriginal.uid != 0, "requires id")
	email_ := emailOriginal
	email_.body = body
	email_.toAddress = emailOriginal.fromAddress
	email_.fromAddress = g_config.Email
	email_.fromName = g_config.DisplayName
	subject := strings.TrimSpace(email_.subject)
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	email_.subject = subject
	sendEmail(email_)
}

func composeEmail(email Email) {
	email.fromAddress = g_config.Email
	email.fromName = g_config.DisplayName
	sendEmail(email)
}

func sendEmail(email Email) {
	chSendMails <- email
}
