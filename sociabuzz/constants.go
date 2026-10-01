// Package sociabuzz — reverse-engineered Go SDK for SociaBuzz donations.
//
// Flow: fees → donate → payment/methods → payment/create → payment/status.
package sociabuzz

// SUPPORTED_COUNTRIES maps country name → currency + ISO2 code.
var SUPPORTED_COUNTRIES = map[string]CountryInfo{
	// Southeast Asia
	"Malaysia":        {Currency: "MYR", ISO2: "MY"},
	"Indonesia":       {Currency: "IDR", ISO2: "ID"},
	"Singapore":       {Currency: "SGD", ISO2: "SG"},
	"Philippines":     {Currency: "PHP", ISO2: "PH"},
	"Thailand":        {Currency: "THB", ISO2: "TH"},
	// South Asia
	"Pakistan":        {Currency: "PKR", ISO2: "PK"},
	"Bangladesh":      {Currency: "BDT", ISO2: "BD"},
	"Sri Lanka":       {Currency: "LKR", ISO2: "LK"},
	// Middle East
	"United Arab Emirates": {Currency: "AED", ISO2: "AE"},
	"Saudi Arabia":    {Currency: "SAR", ISO2: "SA"},
	"Qatar":           {Currency: "QAR", ISO2: "QA"},
	// Africa
	"Nigeria":         {Currency: "NGN", ISO2: "NG"},
	"South Africa":    {Currency: "ZAR", ISO2: "ZA"},
	// Europe
	"United Kingdom":  {Currency: "GBP", ISO2: "GB"},
	"Denmark":         {Currency: "DKK", ISO2: "DK"},
	// Oceania
	"Australia":       {Currency: "AUD", ISO2: "AU"},
	"New Zealand":     {Currency: "NZD", ISO2: "NZ"},
	// Americas
	"United States":   {Currency: "USD", ISO2: "US"},
}

// CountryInfo holds currency and ISO-3166-1 alpha-2 code.
type CountryInfo struct {
	Currency string
	ISO2     string
}

// SupportedCountry returns the CountryInfo for a given country name.
func SupportedCountry(name string) (CountryInfo, bool) {
	info, ok := SUPPORTED_COUNTRIES[name]
	return info, ok
}

// CountryEntry is a named country entry for listing.
type CountryEntry struct {
	Name     string
	Currency string
	ISO2     string
}

// AllCountries returns all supported countries as a sorted slice.
func AllCountries() []CountryEntry {
	entries := make([]CountryEntry, 0, len(SUPPORTED_COUNTRIES))
	for name, info := range SUPPORTED_COUNTRIES {
		entries = append(entries, CountryEntry{Name: name, Currency: info.Currency, ISO2: info.ISO2})
	}
	return entries
}

// GROUP_LABELS maps raw payment channel group names to display labels.
var GROUP_LABELS = map[string]string{
	"ewallet":            "E-Wallet",
	"ewallet_id":         "E-Wallet",
	"ewallet_all":        "E-Wallet",
	"qris":               "QRIS",
	"bank_transfer":      "Bank Transfer",
	"bank_transfer_inter": "Bank Transfer",
	"direct_debit":       "Direct Debit",
	"bank_redirect":      "Bank Redirect",
	"retail_outlet":      "Retail Outlet",
	"retail_outlet_ph":   "Retail Outlet",
	"credit_card":        "Card",
	"credit_card_stripe": "Card",
	"crypto":             "Crypto",
}