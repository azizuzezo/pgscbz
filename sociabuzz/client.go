// Package sociabuzz — high-level Go SDK for SociaBuzz donations.
//
// Usage:
//
//	sb := sociabuzz.New("https://sociabuzz.com")
//	donation, err := sb.Donate(ctx, "creator", 5.0, "Malaysia", "John", "john@test.com", "")
//	methods, err := donation.GetPaymentMethods(ctx)
//	payment, err := donation.CreatePayment(ctx, "my_touchngo")
//	status, err := payment.CheckStatus(ctx)
//	donation.Close()
package sociabuzz

import (
	"context"
	"fmt"
	"strings"
)

// SociaBuzz is the entry point for the donation SDK.
type SociaBuzz struct {
	baseURL string
}

// New creates a new SociaBuzz client.
func New(baseURL string) *SociaBuzz {
	if baseURL == "" {
		baseURL = "https://sociabuzz.com"
	}
	return &SociaBuzz{baseURL: strings.TrimRight(baseURL, "/")}
}

// Donate submits a donation and returns a Donation for the rest of the flow.
func (sb *SociaBuzz) Donate(ctx context.Context, username string, amount float64, country, fullname, email, note string) (*Donation, error) {
	info, ok := SUPPORTED_COUNTRIES[country]
	if !ok {
		supported := make([]string, 0, len(SUPPORTED_COUNTRIES))
		for k := range SUPPORTED_COUNTRIES {
			supported = append(supported, k)
		}
		return nil, &UnsupportedCountryError{Country: country, Supported: supported}
	}
	currency := info.Currency

	httpClient := NewHTTPClient(sb.baseURL)

	_, _, err := httpClient.InitSession(username)
	if err != nil {
		httpClient.Close()
		return nil, &DonationError{Message: fmt.Sprintf("SociaBuzz request failed: %v", err)}
	}

	result, err := httpClient.SubmitDonation(username, currency, amount, fullname, email, note)
	if err != nil {
		httpClient.Close()
		return nil, &DonationError{Message: fmt.Sprintf("SociaBuzz request failed: %v", err)}
	}

	success := false
	switch s := result.Success.(type) {
	case bool:
		success = s
	case string:
		success = s == "true"
	}
	if !success {
		httpClient.Close()
		return nil, &DonationError{
			Message: "Donation validation failed",
			Errors:  result.Validates,
		}
	}

	var token string
	var redirectURL string
	var expiresIn int = 3600
	if result.Content != nil {
		redirectURL = result.Content.Redirect
		expiresIn = result.Content.ExpiredAt
		if expiresIn <= 0 {
			expiresIn = 3600
		}
		// Extract token from redirect URL
		if redirectURL != "" {
			redirectURL = strings.TrimRight(redirectURL, "/")
			parts := strings.Split(redirectURL, "/")
			if len(parts) > 0 {
				token = parts[len(parts)-1]
			}
		}
	}

	if token == "" {
		return nil, &DonationError{Message: "Failed to obtain payment token"}
	}

	donation := &Donation{
		http:        httpClient,
		Token:       token,
		Country:     country,
		Currency:    currency,
		Amount:      result.ActualAmount,
		RedirectURL: redirectURL,
		ExpiresIn:   expiresIn,
	}
	return donation, nil
}

// Donation represents an active donation session.
type Donation struct {
	http        *HTTPClient
	Token       string
	Country     string
	Currency    string
	Amount      float64
	RedirectURL string
	ExpiresIn   int
	csrf        string
	settings    *paymentSettingsResponse
	CallbackURL string
}

// GetPaymentMethods returns every available payment method with fee details.
func (d *Donation) GetPaymentMethods(ctx context.Context) ([]PaymentMethod, error) {
	if err := d.ensureCSRF(ctx); err != nil {
		return nil, err
	}
	settings, err := d.fetchSettings(ctx)
	if err != nil {
		return nil, err
	}

	var methods []PaymentMethod
	counter := 0
	for _, group := range settings.PaymentChannel {
		rawGroup := group.Name
		for _, m := range group.Available {
			if !m.Active {
				continue
			}
			counter++
			methods = append(methods, PaymentMethod{
				ID:      counter,
				Name:    m.Name,
				Label:   ifEmpty(m.Alias, m.Name),
				Group:   groupLabel(rawGroup),
				Gateway: m.Source,
				Fee:     buildFee(m),
			})
		}
	}
	return methods, nil
}

// CreatePayment creates a payment and returns a Payment object.
func (d *Donation) CreatePayment(ctx context.Context, methodName string) (*Payment, error) {
	if err := d.ensureCSRF(ctx); err != nil {
		return nil, err
	}
	settings, err := d.fetchSettings(ctx)
	if err != nil {
		return nil, err
	}

	methodInfo, rawGroup := findMethod(settings, methodName)
	if methodInfo == nil {
		return nil, &PaymentError{Message: fmt.Sprintf(
			"payment method '%s' not available. call GetPaymentMethods() to see valid options", methodName)}
	}

	finalCurrency := d.Currency
	if methodName == "qris" {
		finalCurrency = "IDR"
	}
	if methodInfo.Source == "2c2p" && rawGroup == "credit_card" {
		finalCurrency = "IDR"
	}

	countryInfo := SUPPORTED_COUNTRIES[d.Country]
	iso2 := countryInfo.ISO2
	if iso2 == "" {
		iso2 = d.Country[:2]
	}

	result, err := d.http.CreatePayment(
		d.Token, d.csrf, methodName, rawGroup, methodInfo.Source,
		finalCurrency, strings.ToUpper(iso2), d.Country,
	)
	if err != nil {
		return nil, &PaymentError{Message: err.Error()}
	}

	var paymentURL, qrString string
	if result.Data != nil {
		paymentURL = result.Data.PaymentLink
		if paymentURL == "" {
			paymentURL = result.Data.RedirectURL
		}
		qrString = result.Data.QRString
	}
	if paymentURL == "" {
		if r, ok := result.Redirect.(string); ok && r != "" {
			paymentURL = r
		}
	}

	payment := &Payment{
		http:       d.http,
		Token:      d.Token,
		Method:     methodName,
		Label:      ifEmpty(methodInfo.Alias, methodName),
		Gateway:    methodInfo.Source,
		Fee:        buildFee(*methodInfo),
		PaymentURL: paymentURL,
		QRString:   qrString,
		InvID:      result.InvID,
	}
	return payment, nil
}

// CheckPaymentStatus checks payment status directly from the donation.
func (d *Donation) CheckPaymentStatus(ctx context.Context, paymentMethod, invID string) (string, error) {
	return d.http.CheckPaymentStatus(d.Token, paymentMethod, invID)
}

// Close closes the underlying HTTP session.
func (d *Donation) Close() {
	if d.http != nil {
		d.http.Close()
	}
}

// Serialize snapshots the donation state for persistence.
func (d *Donation) Serialize() map[string]any {
	return map[string]any{
		"token":        d.Token,
		"country":      d.Country,
		"currency":     d.Currency,
		"amount":       d.Amount,
		"redirect_url": d.RedirectURL,
		"expires_in":   d.ExpiresIn,
		"csrf":         d.csrf,
		"callback_url": d.CallbackURL,
		"cookies":      d.http.SerializeCookies(),
	}
}

// Restore rebuilds a Donation from a previously serialised snapshot.
func (d *Donation) Restore(data map[string]any) {
	if d.http == nil {
		d.http = NewHTTPClient("")
	}
	if cookies, ok := data["cookies"].(map[string]string); ok {
		d.http.RestoreCookies(cookies)
	}
	d.Token, _ = data["token"].(string)
	d.Country, _ = data["country"].(string)
	d.Currency, _ = data["currency"].(string)
	d.Amount, _ = data["amount"].(float64)
	d.RedirectURL, _ = data["redirect_url"].(string)
	if ei, ok := data["expires_in"].(int); ok {
		d.ExpiresIn = ei
	}
	d.csrf, _ = data["csrf"].(string)
	d.CallbackURL, _ = data["callback_url"].(string)
}

// SetCSRF sets the CSRF token (used by server session restore).
func (d *Donation) SetCSRF(csrf string) {
	d.csrf = csrf
}

// GetCSRF returns the current CSRF token.
func (d *Donation) GetCSRF() string {
	return d.csrf
}

// HTTPClient returns the underlying HTTP client (for cookie serialization).
func (d *Donation) HTTPClient() *HTTPClient {
	return d.http
}

func (d *Donation) ensureCSRF(ctx context.Context) error {
	if d.csrf != "" {
		return nil
	}
	csrf, err := d.http.VisitPaymentPage(d.Token)
	if err != nil {
		return err
	}
	d.csrf = csrf
	return nil
}

func (d *Donation) fetchSettings(ctx context.Context) (*paymentSettingsResponse, error) {
	if d.settings != nil {
		return d.settings, nil
	}
	settings, err := d.http.GetPaymentSettings(d.Token, d.Amount, d.Currency, d.Country)
	if err != nil {
		return nil, err
	}
	d.settings = settings
	return settings, nil
}

// Payment represents a created payment with URL/QR code.
type Payment struct {
	http       *HTTPClient
	Token      string
	Method     string
	Label      string
	Gateway    string
	Fee        FeeInfo
	PaymentURL string
	QRString   string
	InvID      string
}

// CheckStatus polls the payment status. Returns "pending", "paid", "expired", or "failed".
func (p *Payment) CheckStatus(ctx context.Context) (string, error) {
	invID := p.InvID
	if invID == "" {
		invID = p.Token
	}
	return p.http.CheckPaymentStatus(p.Token, p.Method, invID)
}

// ── internal helpers ──────────────────────────────────────────────────

func buildFee(m paymentMethodRaw) FeeInfo {
	totalFee := fmt.Sprintf("%v", m.TotalFee)
	totalPay := fmt.Sprintf("%v", m.TotalPay)
	// Ensure string values
	return FeeInfo{
		FeeType:   m.FeeType,
		FeeRate:   m.Fee,
		FeeFixed:  m.FeeAmount,
		TotalFee:  totalFee,
		TotalPay:  totalPay,
		MinAmount: m.MinAmount,
		MaxAmount: m.MaxAmount,
	}
}

func findMethod(settings *paymentSettingsResponse, name string) (*paymentMethodRaw, string) {
	for _, group := range settings.PaymentChannel {
		for _, m := range group.Available {
			if m.Name == name {
				method := m
				return &method, group.Name
			}
		}
	}
	return nil, ""
}

func groupLabel(raw string) string {
	if label, ok := GROUP_LABELS[raw]; ok {
		return label
	}
	return raw
}

func ifEmpty(a, b string) string {
	if a == "" {
		return b
	}
	return a
}