package sociabuzz

import (
	"context"
	"fmt"
	"strings"
)

// FeeCalculator fetches live fee estimates for a country + amount.
type FeeCalculator struct {
	http     *HTTPClient
	username string
}

// NewFeeCalculator creates a new fee calculator.
func NewFeeCalculator(baseURL, username string) *FeeCalculator {
	if baseURL == "" {
		baseURL = "https://sociabuzz.com"
	}
	return &FeeCalculator{
		http:     NewHTTPClient(baseURL),
		username: username,
	}
}

// Estimate returns fee estimates for every active method in the given country.
func (fc *FeeCalculator) Estimate(ctx context.Context, country string, amount float64, username string) ([]FeeEstimate, error) {
	info, ok := SUPPORTED_COUNTRIES[country]
	if !ok {
		supported := make([]string, 0, len(SUPPORTED_COUNTRIES))
		for k := range SUPPORTED_COUNTRIES {
			supported = append(supported, k)
		}
		return nil, &UnsupportedCountryError{Country: country, Supported: supported}
	}
	currency := info.Currency
	uname := username
	if uname == "" {
		uname = fc.username
	}

	_, _, err := fc.http.InitSession(uname)
	if err != nil {
		return nil, fmt.Errorf("fee estimate: init session: %w", err)
	}

	result, err := fc.http.SubmitDonation(uname, currency, amount, "Fee Estimate", "noreply@sociabuzz-sdk.local", "")
	if err != nil {
		return nil, fmt.Errorf("fee estimate: donation: %w", err)
	}

	success := false
	switch s := result.Success.(type) {
	case bool:
		success = s
	case string:
		success = s == "true"
	}
	if !success {
		return nil, &DonationError{
			Message: "Donation validation failed during fee estimate",
			Errors:  result.Validates,
		}
	}

	var token string
	if result.Content != nil && result.Content.Redirect != "" {
		redirectURL := strings.TrimRight(result.Content.Redirect, "/")
		parts := strings.Split(redirectURL, "/")
		if len(parts) > 0 {
			token = parts[len(parts)-1]
		}
	}
	if token == "" {
		return nil, fmt.Errorf("fee estimate: failed to obtain payment token")
	}

	actualAmount := result.ActualAmount
	if actualAmount == 0 {
		actualAmount = amount
	}

	_, err = fc.http.VisitPaymentPage(token)
	if err != nil {
		return nil, fmt.Errorf("fee estimate: visit payment page: %w", err)
	}

	settings, err := fc.http.GetPaymentSettings(token, actualAmount, currency, country)
	if err != nil {
		return nil, fmt.Errorf("fee estimate: get payment settings: %w", err)
	}

	return parseSettings(settings, currency, actualAmount), nil
}

// Close closes the underlying HTTP session.
func (fc *FeeCalculator) Close() {
	fc.http.Close()
}

// DefaultAmounts is the sensible default donation amounts per currency.
var DefaultAmounts = map[string]float64{
	"MYR": 5.0, "IDR": 10000.0, "SGD": 5.0, "PHP": 100.0, "THB": 100.0,
	"PKR": 500.0, "BDT": 500.0, "LKR": 1000.0,
	"AED": 20.0, "SAR": 20.0, "QAR": 20.0,
	"NGN": 2000.0, "ZAR": 50.0,
	"GBP": 5.0, "DKK": 50.0,
	"AUD": 5.0, "NZD": 5.0,
	"USD": 5.0,
}

// EstimateAll returns fee estimates for every supported country.
// amountOverrides optionally overrides amounts per currency.
func (fc *FeeCalculator) EstimateAll(ctx context.Context, amountOverrides map[string]float64, username string) map[string][]FeeEstimate {
	amounts := make(map[string]float64, len(DefaultAmounts))
	for k, v := range DefaultAmounts {
		amounts[k] = v
	}
	for k, v := range amountOverrides {
		amounts[k] = v
	}

	uname := username
	if uname == "" {
		uname = fc.username
	}

	results := make(map[string][]FeeEstimate, len(SUPPORTED_COUNTRIES))
	for country, info := range SUPPORTED_COUNTRIES {
		amt := amounts[info.Currency]
		if amt == 0 {
			amt = 5.0
		}
		est, err := fc.Estimate(ctx, country, amt, uname)
		if err != nil {
			results[country] = nil
		} else {
			results[country] = est
		}
	}
	return results
}

// ── internal ─────────────────────────────────────────────────────────

func parseSettings(settings *paymentSettingsResponse, currency string, amount float64) []FeeEstimate {
	var estimates []FeeEstimate
	counter := 0
	for _, group := range settings.PaymentChannel {
		rawGroup := group.Name
		for _, m := range group.Available {
			if !m.Active {
				continue
			}
			counter++
			estimates = append(estimates, FeeEstimate{
				ID:       counter,
				Name:     m.Name,
				Label:    ifEmpty(m.Alias, m.Name),
				Group:    groupLabel(rawGroup),
				Gateway:  m.Source,
				Fee:      buildFee(m),
				Currency: currency,
				Amount:   amount,
			})
		}
	}
	return estimates
}