package main

import (
	"fmt"

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
			Require(email.id != 0, "requires id")
			msg := mail.NewMessage()
			msg.SetHeader(
				"From",
				msg.FormatAddress(email.fromAddress, email.fromName),
			)
			msg.SetHeader("To", email.toAddress)
			msg.SetHeader("Subject", email.subject)
			msg.SetBody("text/plain", email.body)

			dialer := mail.NewDialer(g_config.SMTPHost, 465,
				g_config.Email, g_config.Password)
			err := dialer.DialAndSend(msg)
			if err != nil {
				updateStatusBar(fmt.Sprintf("couldn't send email: %v", err))
				break
			}

			updateStatusBar(fmt.Sprintf("email sent to: %s", email.toAddress))
		}
	}
}

func sendEmail(email Email) {
	chSendMails <- email
}
