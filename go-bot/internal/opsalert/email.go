package opsalert

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"
	"time"
)

type EmailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

type EmailNotifier struct {
	cfg EmailConfig
}

func NewEmailNotifier(cfg EmailConfig) (*EmailNotifier, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("smtp host is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("smtp from address is required")
	}
	if len(cfg.To) == 0 {
		return nil, fmt.Errorf("smtp recipients are required")
	}
	return &EmailNotifier{cfg: cfg}, nil
}

func (n *EmailNotifier) Notify(ctx context.Context, alert Alert) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	subject := fmt.Sprintf("[%s] %s", strings.ToUpper(string(alert.Severity)), alert.Title)
	msg, err := buildEmail(n.cfg.From, n.cfg.To, subject, alert)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", n.cfg.Host, n.cfg.Port)
	var auth smtp.Auth
	if n.cfg.Username != "" || n.cfg.Password != "" {
		auth = smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
	}
	return smtp.SendMail(addr, auth, n.cfg.From, n.cfg.To, msg)
}

func buildEmail(from string, to []string, subject string, alert Alert) ([]byte, error) {
	boundary := fmt.Sprintf("opsalert-%d", time.Now().UnixNano())
	text := FormatText(alert)

	var html bytes.Buffer
	if err := emailTemplate.Execute(&html, struct {
		Alert Alert
		Keys  []string
	}{
		Alert: normalize(alert, time.Now()),
		Keys:  sortedKeys(alert.Fields),
	}); err != nil {
		return nil, err
	}

	var msg bytes.Buffer
	msg.WriteString("From: " + from + "\r\n")
	msg.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(`Content-Type: multipart/alternative; boundary="` + boundary + `"` + "\r\n")
	msg.WriteString("\r\n--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	msg.WriteString(text)
	msg.WriteString("\r\n--" + boundary + "\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	msg.Write(html.Bytes())
	msg.WriteString("\r\n--" + boundary + "--\r\n")
	return msg.Bytes(), nil
}

var emailTemplate = template.Must(template.New("opsalert").Parse(`<!doctype html>
<html>
<body style="margin:0;background:#f6f8fb;font-family:Arial,sans-serif;color:#17202a;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f6f8fb;padding:24px;">
    <tr>
      <td align="center">
        <table role="presentation" width="640" cellspacing="0" cellpadding="0" style="max-width:640px;background:#ffffff;border:1px solid #d9e0ea;border-radius:8px;overflow:hidden;">
          <tr>
            <td style="padding:18px 22px;background:#17202a;color:#ffffff;">
              <div style="font-size:12px;letter-spacing:0;text-transform:uppercase;">{{.Alert.Component}}</div>
              <div style="font-size:22px;font-weight:700;margin-top:6px;">{{.Alert.Title}}</div>
            </td>
          </tr>
          <tr>
            <td style="padding:22px;">
              <span style="display:inline-block;padding:6px 10px;border-radius:6px;background:{{if eq .Alert.Severity "critical"}}#ffd9d9{{else if eq .Alert.Severity "warning"}}#fff1c2{{else}}#dff5e7{{end}};color:#17202a;font-weight:700;text-transform:uppercase;font-size:12px;">{{.Alert.Severity}}</span>
              <p style="font-size:15px;line-height:1.5;margin:18px 0;">{{.Alert.Summary}}</p>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="border-collapse:collapse;font-size:14px;">
                {{if .Alert.UserID}}<tr><td style="padding:8px;border-top:1px solid #e8edf4;font-weight:700;">User ID</td><td style="padding:8px;border-top:1px solid #e8edf4;">{{.Alert.UserID}}</td></tr>{{end}}
                {{if .Alert.Symbol}}<tr><td style="padding:8px;border-top:1px solid #e8edf4;font-weight:700;">Symbol</td><td style="padding:8px;border-top:1px solid #e8edf4;">{{.Alert.Symbol}}</td></tr>{{end}}
                {{if .Alert.PositionID}}<tr><td style="padding:8px;border-top:1px solid #e8edf4;font-weight:700;">Position</td><td style="padding:8px;border-top:1px solid #e8edf4;">{{.Alert.PositionID}}</td></tr>{{end}}
                {{if .Alert.Error}}<tr><td style="padding:8px;border-top:1px solid #e8edf4;font-weight:700;">Error</td><td style="padding:8px;border-top:1px solid #e8edf4;">{{.Alert.Error}}</td></tr>{{end}}
                {{range .Keys}}<tr><td style="padding:8px;border-top:1px solid #e8edf4;font-weight:700;">{{.}}</td><td style="padding:8px;border-top:1px solid #e8edf4;">{{index $.Alert.Fields .}}</td></tr>{{end}}
                <tr><td style="padding:8px;border-top:1px solid #e8edf4;font-weight:700;">Time</td><td style="padding:8px;border-top:1px solid #e8edf4;">{{.Alert.OccurredAt.UTC.Format "2006-01-02 15:04:05 UTC"}}</td></tr>
              </table>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))
