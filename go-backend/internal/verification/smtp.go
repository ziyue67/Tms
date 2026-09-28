package verification

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type SMTPSettings struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
	StartTLS bool
	SSL      bool
}

type Message struct {
	To      string
	Subject string
	Body    string
	HTML    bool
}

type Mailer interface {
	Send(SMTPSettings, Message) error
}

type SMTPMailer struct{}

func (SMTPMailer) Send(settings SMTPSettings, message Message) error {
	address := net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var connection net.Conn
	var err error
	if settings.SSL {
		connection, err = tls.DialWithDialer(dialer, "tcp", address, &tls.Config{ServerName: settings.Host, MinVersion: tls.VersionTLS12})
	} else {
		connection, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(20 * time.Second))

	client, err := smtp.NewClient(connection, settings.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if settings.StartTLS {
		if err := client.StartTLS(&tls.Config{ServerName: settings.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if settings.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", settings.Username, settings.Password, settings.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(settings.From); err != nil {
		return err
	}
	if err := client.Rcpt(message.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if err := writeMessage(writer, settings, message); err != nil {
		writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func writeMessage(writer io.Writer, settings SMTPSettings, message Message) error {
	from := (&mail.Address{Name: settings.FromName, Address: settings.From}).String()
	contentType := "text/plain"
	if message.HTML {
		contentType = "text/html"
	}
	headers := []string{
		"From: " + sanitizeHeader(from),
		"To: " + sanitizeHeader(message.To),
		"Subject: " + mime.QEncoding.Encode("UTF-8", sanitizeHeader(message.Subject)),
		"MIME-Version: 1.0",
		"Content-Type: " + contentType + "; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		message.Body,
	}
	buffer := bufio.NewWriter(writer)
	if _, err := buffer.WriteString(strings.Join(headers, "\r\n")); err != nil {
		return err
	}
	return buffer.Flush()
}

func sanitizeHeader(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

func validateSMTP(settings SMTPSettings) error {
	if strings.TrimSpace(settings.Host) == "" {
		return fmt.Errorf("SMTP 尚未配置")
	}
	if settings.Port < 1 || settings.Port > 65535 {
		return fmt.Errorf("SMTP 端口无效")
	}
	if settings.Username == "" || settings.Password == "" {
		return fmt.Errorf("SMTP 用户名和授权码不能为空（QQ 邮箱请填写授权码）")
	}
	if settings.From == "" {
		return fmt.Errorf("发件人地址不能为空")
	}
	if _, err := mail.ParseAddress(settings.From); err != nil {
		return fmt.Errorf("发件人地址无效")
	}
	if settings.SSL && settings.StartTLS {
		return fmt.Errorf("SMTP SSL 与 STARTTLS 不能同时启用")
	}
	return nil
}
