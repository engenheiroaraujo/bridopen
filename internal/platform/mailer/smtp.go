package mailer

import (
	"html"
	"regexp"
	"strings"

	"gopkg.in/gomail.v2"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type mailDialer interface {
	DialAndSend(m ...*gomail.Message) error
}

type smtpMailService struct {
	from   string
	dialer mailDialer
}

var (
	htmlBreakPattern = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/h[1-6]|/li|/tr)\b[^>]*>`)
	htmlTagPattern   = regexp.MustCompile(`(?s)<[^>]+>`)
	blankLinePattern = regexp.MustCompile(`\n{3,}`)
	newSMTPDialer    = func(host string, port int, username, password string) mailDialer {
		return gomail.NewDialer(host, port, username, password)
	}
)

func NewSMTPMailService(cfg SMTPConfig) MailService {
	return smtpMailService{from: cfg.From, dialer: newSMTPDialer(cfg.Host, cfg.Port, cfg.Username, cfg.Password)}
}

func (ss smtpMailService) Send(msg MailMessage) error {
	m := gomail.NewMessage()
	m.SetHeader("From", ss.from)
	m.SetHeader("To", msg.To...)
	m.SetHeader("Subject", msg.Subject)

	body := string(msg.Body)
	if msg.IsHtml {
		m.SetBody("text/plain", htmlToPlainText(body))
		m.AddAlternative("text/html", body)
	} else {
		m.SetBody("text/plain", body)
	}

	return ss.dialer.DialAndSend(m)
}

func htmlToPlainText(body string) string {
	text := htmlBreakPattern.ReplaceAllString(body, "\n")
	text = htmlTagPattern.ReplaceAllString(text, "")
	text = html.UnescapeString(text)

	lines := strings.Split(text, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			clean = append(clean, line)
		}
	}

	text = strings.Join(clean, "\n")
	text = blankLinePattern.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
