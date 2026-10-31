package notify

import (
	"strings"
	"time"

	mail "github.com/wneessen/go-mail"
)

type EmailConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	FromName    string `json:"from_name"`
	FromAddress string `json:"from_address"`
	Security    string `json:"security"`
	TimeoutSec  int    `json:"timeout_sec"`
}

func SendEmail(cfg EmailConfig, to, subject, body string) error {
	return SendEmailHTML(cfg, to, subject, body, "")
}

// SendEmailHTML sends a plain-text body with an optional HTML alternative.
// The text part keeps notifications readable in clients that block HTML.
func SendEmailHTML(cfg EmailConfig, to, subject, textBody, htmlBody string) error {
	security := strings.ToLower(strings.TrimSpace(cfg.Security))
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	opts := []mail.Option{
		mail.WithPort(cfg.Port),
		mail.WithTimeout(timeout),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(cfg.Username),
		mail.WithPassword(cfg.Password),
	}
	switch security {
	case "ssl":
		opts = append(opts, mail.WithSSL())
	case "starttls":
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	}
	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return err
	}
	msg := mail.NewMsg()
	if cfg.FromName != "" {
		if err := msg.FromFormat(cfg.FromName, cfg.FromAddress); err != nil {
			return err
		}
	} else if err := msg.From(cfg.FromAddress); err != nil {
		return err
	}
	if err := msg.To(to); err != nil {
		return err
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextPlain, textBody)
	if strings.TrimSpace(htmlBody) != "" {
		msg.AddAlternativeString(mail.TypeTextHTML, htmlBody)
	}
	return client.DialAndSend(msg)
}
