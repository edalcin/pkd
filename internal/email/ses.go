package email

import (
	"fmt"
	"net/smtp"
)

// Sender delivers mail through Amazon SES SMTP (STARTTLS on the configured port).
type Sender struct {
	Host, Port, Username, Password, From string
}

// Send delivers a plain-text UTF-8 message to a single recipient.
func (s Sender) Send(to, subject, body string) error {
	auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n",
		s.From, to, subject, body))
	return smtp.SendMail(s.Host+":"+s.Port, auth, s.From, []string{to}, msg)
}
