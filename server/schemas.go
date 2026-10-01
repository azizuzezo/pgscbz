package server

import (
	"encoding/json"
	"net/http"
	"time"
)

// ── Step 1 — Donate ──────────────────────────────────────────────────

type DonateRequest struct {
	Username    string  `json:"username"`
	Amount      float64 `json:"amount"`
	Country     string  `json:"country"`
	Fullname    string  `json:"fullname"`
	Email       string  `json:"email"`
	Note        string  `json:"note"`
	CallbackURL string  `json:"callback_url"`
}

type DonateResponse struct {
	Success     bool    `json:"success"`
	Token       string  `json:"token"`
	Country     string  `json:"country"`
	Currency    string  `json:"currency"`
	Amount      float64 `json:"amount"`
	RedirectURL string  `json:"redirect_url"`
	ExpiresIn   int     `json:"expires_in"`
}

// ── Step 2 — Payment Methods ─────────────────────────────────────────

type PaymentMethodsRequest struct {
	Token string `json:"token"`
}

type PaymentMethodItem struct {
	ID      int                `json:"id"`
	Name    string             `json:"name"`
	Label   string             `json:"label"`
	Group   string             `json:"group"`
	Gateway string             `json:"gateway"`
	Fee     SociaFeeInfo       `json:"fee"`
}

type PaymentMethodsResponse struct {
	Success  bool                `json:"success"`
	Token    string              `json:"token"`
	Country  string              `json:"country"`
	Currency string              `json:"currency"`
	Amount   float64             `json:"amount"`
	Methods  []PaymentMethodItem `json:"methods"`
	Total    int                 `json:"total"`
}

// ── Step 3 — Create Payment ──────────────────────────────────────────

type CreatePaymentRequest struct {
	Token         string `json:"token"`
	PaymentMethod string `json:"payment_method"`
}

type CreatePaymentResponse struct {
	Success       bool          `json:"success"`
	PaymentMethod string        `json:"payment_method"`
	Label         string        `json:"label"`
	Gateway       string        `json:"gateway"`
	Fee           SociaFeeInfo  `json:"fee"`
	PaymentURL    *string       `json:"payment_url,omitempty"`
	QRString      *string       `json:"qr_string,omitempty"`
	Token         string        `json:"token"`
	InvID         *string       `json:"inv_id,omitempty"`
}

// ── Step 4 — Payment Status ──────────────────────────────────────────

type PaymentStatusRequest struct {
	Token         string `json:"token"`
	InvID         string `json:"inv_id"`
	PaymentMethod string `json:"payment_method"`
}

type PaymentStatusResponse struct {
	Success bool   `json:"success"`
	Token   string `json:"token"`
	Status  string `json:"status"`
}

// ── Fees ─────────────────────────────────────────────────────────────

type FeesRequest struct {
	Country  string  `json:"country"`
	Amount   float64 `json:"amount"`
	Username string  `json:"username"`
}

type FeeMethodItem struct {
	ID       int           `json:"id"`
	Name     string        `json:"name"`
	Label    string        `json:"label"`
	Group    string        `json:"group"`
	Gateway  string        `json:"gateway"`
	Currency string        `json:"currency"`
	Amount   float64       `json:"amount"`
	Fee      SociaFeeInfo  `json:"fee"`
}

type FeesResponse struct {
	Success  bool            `json:"success"`
	Country  string          `json:"country"`
	Currency string          `json:"currency"`
	Amount   float64         `json:"amount"`
	Methods  []FeeMethodItem `json:"methods"`
	Total    int             `json:"total"`
}

type CountriesItem struct {
	Country  string `json:"country"`
	Currency string `json:"currency"`
	ISO2     string `json:"iso2"`
}

type CountriesResponse struct {
	Success   bool            `json:"success"`
	Total     int             `json:"total"`
	Countries []CountriesItem `json:"countries"`
}

// SociaFeeInfo mirrors sociabuzz.FeeInfo but with json tags for the API.
type SociaFeeInfo struct {
	FeeType   string  `json:"fee_type"`
	FeeRate   float64 `json:"fee_rate"`
	FeeFixed  float64 `json:"fee_fixed"`
	TotalFee  string  `json:"total_fee"`
	TotalPay  string  `json:"total_pay"`
	MinAmount float64 `json:"min_amount"`
	MaxAmount float64 `json:"max_amount"`
}

// ── Helpers ──────────────────────────────────────────────────────────

// JSONError writes a JSON error response.
func JSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// JSONOK writes a JSON success response.
func JSONOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// Ensure time is imported (used by session)
var _ = time.Now