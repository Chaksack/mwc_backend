package email

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/mail"
	"strings"

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

// parseEmailAddress parses and formats an email address to be RFC 5322 compliant
func parseEmailAddress(emailAddr string) (string, error) {
	// Remove any surrounding quotes that might cause issues
	cleanAddr := strings.Trim(emailAddr, `"`)
	
	// Try to parse the address using Go's mail package
	addr, err := mail.ParseAddress(cleanAddr)
	if err != nil {
		// If parsing fails, try with the original address
		addr, err = mail.ParseAddress(emailAddr)
		if err != nil {
			// If both fail, return the original address and log a warning
			log.Printf("Warning: Could not parse email address '%s': %v. Using as-is.", emailAddr, err)
			return emailAddr, nil
		}
	}
	
	// Return the properly formatted address
	return addr.String(), nil
}

// NewGoMailerService creates a new GoMailerService.
func NewGoMailerService(host string, port int, username, password, from string) EmailService {
	if host == "" || port == 0 || from == "" {
		log.Println("Warning: SMTP host, port, or fromAddress not configured. Email service will be a no-op.")
		return &noopEmailService{} // Return a no-op service
	}
	
	// Parse and format the from address
	formattedFrom, err := parseEmailAddress(from)
	if err != nil {
		log.Printf("Warning: Email from address formatting issue: %v", err)
	}
	
	d := gomail.NewDialer(host, port, username, password)
	// Ensure TLS is configured properly for SES/SMTP
	d.TLSConfig = &tls.Config{ServerName: host}
	return &GoMailerService{dialer: d, fromAddr: formattedFrom}
}

// NewNoopEmailService returns a no-op email service (useful for dev/sandbox)
func NewNoopEmailService() EmailService {
	return &noopEmailService{}
}

// SendEmail sends an email.
func (s *GoMailerService) SendEmail(to, subject, htmlBody string) error {
	// Dialer and fromAddr are checked in NewGoMailerService implicitly
	// by returning noopEmailService if not configured.

	m := gomail.NewMessage()
	m.SetHeader("From", s.fromAddr)
	// Format recipient address
	formattedTo, err := parseEmailAddress(to)
	if err != nil {
		log.Printf("Warning: Email 'to' address formatting issue: %v", err)
		formattedTo = to
	}
	m.SetHeader("To", formattedTo)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)
	// m.AddAlternative("text/plain", "Plain text version of the email...") // Good practice

	if err := s.dialer.DialAndSend(m); err != nil {
		// Provide SES sandbox friendly hint if applicable
		errStr := err.Error()
		if strings.Contains(errStr, "554") && strings.Contains(errStr, "Email address is not verified") {
			log.Printf("Notice: SES rejected email to %s because the address is not verified (sandbox). Consider verifying the recipient or requesting production access, or set EMAIL_ENABLED=false for dev.", formattedTo)
		}
		return fmt.Errorf("could not send email to %s: %w", formattedTo, err)
	}
	log.Printf("Email sent successfully to %s, Subject: %s", formattedTo, subject)
	return nil
}

// noopEmailService is an EmailService that does nothing, used when SMTP is not configured.
type noopEmailService struct{}

func (s *noopEmailService) SendEmail(to, subject, htmlBody string) error {
	log.Printf("Email service is not configured. Would have sent email to %s with subject '%s'", to, subject)
	return nil // Do not error, just log
}
