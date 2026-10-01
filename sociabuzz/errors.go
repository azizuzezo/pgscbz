package sociabuzz

import "fmt"

// SociaBuzzError is the base error type.
type SociaBuzzError struct {
	Message string
}

func (e *SociaBuzzError) Error() string { return e.Message }

// UnsupportedCountryError is raised for unsupported countries.
type UnsupportedCountryError struct {
	Country   string
	Supported []string
}

func (e *UnsupportedCountryError) Error() string {
	return fmt.Sprintf("unsupported country '%s'. choose from: %v", e.Country, e.Supported)
}

// DonationError is raised when donation submission fails.
type DonationError struct {
	Message string
	Errors  []string
}

func (e *DonationError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("%s: %v", e.Message, e.Errors)
	}
	return e.Message
}

// PaymentError is raised when payment creation or status check fails.
type PaymentError struct {
	Message string
}

func (e *PaymentError) Error() string { return e.Message }

// SessionExpiredError is raised when a token is not found or expired.
type SessionExpiredError struct {
	Token string
}

func (e *SessionExpiredError) Error() string {
	return fmt.Sprintf("session not found for token '%s'", e.Token)
}