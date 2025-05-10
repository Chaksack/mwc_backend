package email

import (
	"fmt"
	"log"
	// "strconv" // No longer needed here

	"gopkg.in/gomail.v2"
)

// EmailService defines the interface for sending emails.
type EmailService interface {
	SendEmail(to, subject, htmlBody string) error
}

// GoMailerService implements EmailService using gomail.
type GoMailerService struct {
	dialer   *gomail.Dialer
	fromAddr string
}

// NewGoMailerService creates a new GoMailerService.
func NewGoMailerService(host string, port int, username, password, from string) EmailService {
	if host == "" || port == 0 || from == "" {
		log.Println("Warning: SMTP host, port, or fromAddress not configured. Email service will be a no-op.")
		return &noopEmailService{} // Return a no-op service
	}
	d := gomail.NewDialer(host, port, username, password)
	// TODO: Add d.TLSConfig for TLS, especially if not using standard port 465 (SMTPS) or 587 (STARTTLS)
	return &GoMailerService{dialer: d, fromAddr: from}
}

// SendEmail sends an email.
func (s *GoMailerService) SendEmail(to, subject, htmlBody string) error {
	// Dialer and fromAddr are checked in NewGoMailerService implicitly
	// by returning noopEmailService if not configured.

	m := gomail.NewMessage()
	m.SetHeader("From", s.fromAddr)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)
	// m.AddAlternative("text/plain", "Plain text version of the email...") // Good practice

	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("could not send email to %s: %w", to, err)
	}
	log.Printf("Email sent successfully to %s, Subject: %s", to, subject)
	return nil
}

// noopEmailService is an EmailService that does nothing, used when SMTP is not configured.
type noopEmailService struct{}

func (s *noopEmailService) SendEmail(to, subject, htmlBody string) error {
	log.Printf("Email service is not configured. Would have sent email to %s with subject '%s'", to, subject)
	return nil // Do not error, just log
}
