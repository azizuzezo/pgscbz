package sociabuzz

// FeeInfo is the fee breakdown for a payment method.
type FeeInfo struct {
	FeeType  string  `json:"fee_type"`
	FeeRate  float64 `json:"fee_rate"`
	FeeFixed float64 `json:"fee_fixed"`
	TotalFee string  `json:"total_fee"`
	TotalPay string  `json:"total_pay"`
	MinAmount float64 `json:"min_amount"`
	MaxAmount float64 `json:"max_amount"`
}

// PaymentMethod is an available payment method with fee details.
type PaymentMethod struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Label   string  `json:"label"`
	Group   string  `json:"group"`
	Gateway string  `json:"gateway"`
	Fee     FeeInfo `json:"fee"`
}

// FeeEstimate is a fee quote for one payment method at a given amount.
type FeeEstimate struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	Group    string  `json:"group"`
	Gateway  string  `json:"gateway"`
	Fee      FeeInfo `json:"fee"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// --- Internal HTTP response shapes (from SociaBuzz API) ---

// submitDonationResponse is the parsed response from POST /donate/get-form-queue.
type submitDonationResponse struct {
	Success  any               `json:"success"` // "true" string or bool
	Content  *submitContent    `json:"content,omitempty"`
	Messages map[string]string `json:"messages,omitempty"`
	Validates []string         `json:"validates,omitempty"`
	// Internal fields injected by us.
	ActualAmount  float64
	TraktirPrice  float64
	Qty           int
}

type submitContent struct {
	Redirect  string `json:"redirect,omitempty"`
	ExpiredAt int    `json:"expired_at,omitempty"`
}

// paymentSettingsResponse is the response from GET /payment/pay/setting.
type paymentSettingsResponse struct {
	PaymentChannel []paymentChannel `json:"payment_channel"`
}

type paymentChannel struct {
	Name      string                `json:"name"`
	Available []paymentMethodRaw    `json:"available"`
}

type paymentMethodRaw struct {
	Name       string  `json:"name"`
	Alias      string  `json:"alias,omitempty"`
	Active     bool    `json:"active"`
	Source     string  `json:"source,omitempty"`
	FeeType    string  `json:"fee_type,omitempty"`
	Fee        float64 `json:"fee,omitempty"`
	FeeAmount  float64 `json:"fee_amount,omitempty"`
	TotalFee   any     `json:"total_fee,omitempty"`   // string or number
	TotalPay   any     `json:"total_pay,omitempty"`   // string or number
	MinAmount  float64 `json:"min_amount,omitempty"`
	MaxAmount  float64 `json:"max_amount,omitempty"`
}

// createPaymentResponse is the response from POST /payment/send/create.
type createPaymentResponse struct {
	Data     *paymentData `json:"data,omitempty"`
	Redirect any          `json:"redirect,omitempty"` // string or bool
	InvID    string       `json:"inv_id,omitempty"`
}

type paymentData struct {
	PaymentLink string `json:"payment_link,omitempty"`
	RedirectURL string `json:"redirect_url,omitempty"`
	QRString    string `json:"qr_string,omitempty"`
}