// Package sociabuzz implements a reverse-engineered Go SDK for SociaBuzz donations.
//
// This file contains the low-level HTTP client that manages cookies, CSRF tokens,
// and the full SociaBuzz request lifecycle.
package sociabuzz

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// commonHeaders are sent with every request to mimic a real browser.
var commonHeaders = http.Header{
	"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"},
	"Accept-Language": {"en-US,en;q=0.9"},
	"Sec-Ch-Ua":       {`"Chromium";v="146", "Not-A.Brand";v="24", "Google Chrome";v="146"`},
	"Sec-Ch-Ua-Mobile": {"?0"},
	"Sec-Ch-Ua-Platform": {`"Windows"`},
}

// HTTPClient is the low-level HTTP client that manages cookies and CSRF tokens.
type HTTPClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPClient creates a new SociaBuzz HTTP client.
func NewHTTPClient(baseURL string) *HTTPClient {
	if baseURL == "" {
		baseURL = "https://sociabuzz.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	jar, _ := cookiejar.New(nil)
	return &HTTPClient{
		baseURL: baseURL,
		client: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
}

// SerializeCookies snapshots the current cookie jar as a plain map.
// If duplicate cookie names exist, the last value wins.
func (h *HTTPClient) SerializeCookies() map[string]string {
	u, _ := url.Parse(h.baseURL)
	cookies := h.client.Jar.Cookies(u)
	result := make(map[string]string, len(cookies))
	for _, c := range cookies {
		result[c.Name] = c.Value
	}
	return result
}

// RestoreCookies restores a previously serialised cookie jar into the client.
func (h *HTTPClient) RestoreCookies(cookies map[string]string) {
	if len(cookies) == 0 {
		return
	}
	u, _ := url.Parse(h.baseURL)
	var cls []*http.Cookie
	for name, value := range cookies {
		cls = append(cls, &http.Cookie{Name: name, Value: value})
	}
	h.client.Jar.SetCookies(u, cls)
}

// ── helpers ──────────────────────────────────────────────────────────

func (h *HTTPClient) url(path string) string {
	return h.baseURL + path
}

func (h *HTTPClient) do(req *http.Request) (*http.Response, error) {
	for k, v := range commonHeaders {
		// Don't overwrite caller-set headers
		if req.Header.Get(k) == "" {
			req.Header[k] = v
		}
	}
	return h.client.Do(req)
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ── Step 0: init session ─────────────────────────────────────────────

// InitSession GETs /{username}/tribe to seed ci_session + CSRF cookies.
// Returns a map of cookies and the CSRF token.
func (h *HTTPClient) InitSession(username string) (cookies map[string]string, csrfToken string, err error) {
	req, err := http.NewRequest(http.MethodGet, h.url("/"+username+"/tribe"), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")

	resp, err := h.do(req)
	if err != nil {
		return nil, "", fmt.Errorf("init_session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("init_session: HTTP %d", resp.StatusCode)
	}
	io.Copy(io.Discard, resp.Body) // drain body

	u, _ := url.Parse(h.baseURL)
	cks := h.client.Jar.Cookies(u)
	cookies = make(map[string]string, len(cks))
	for _, c := range cks {
		cookies[c.Name] = c.Value
	}
	csrfToken = cookies["sociabuzz_sb_cookie_csrf"]
	if csrfToken == "" {
		csrfToken = cookies["csrf_cookie_name"]
	}
	return cookies, csrfToken, nil
}

// ── Step 0.5: creator config ─────────────────────────────────────────

var reClientConfig = regexp.MustCompile(`var\s+client\s*=\s*(\{[^;]+\})\s*;`)

// GetCreatorConfig GETs /{username}/donate/queue and extracts the client config JSON.
// Returns a map with keys like "TRAKTIR_PRICE".
func (h *HTTPClient) GetCreatorConfig(username, currency string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, h.url("/"+username+"/donate/queue"), nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("type", "buyme")
	q.Set("currency", currency)
	q.Set("message", "")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", h.url("/"+username+"/tribe"))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")

	resp, err := h.do(req)
	if err != nil {
		return nil, fmt.Errorf("get_creator_config: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get_creator_config: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	m := reClientConfig.FindSubmatch(body)
	if m == nil {
		return map[string]any{}, nil
	}
	return parseJSONMap(string(m[1]))
}

// ── Step 1: submit donation ──────────────────────────────────────────

// SubmitDonation POSTs the donation form and returns the parsed response.
func (h *HTTPClient) SubmitDonation(username, currency string, amount float64, fullname, email, note string) (*submitDonationResponse, error) {
	// Ensure CSRF cookie exists
	csrfToken := h.getCSRFCookie()
	if csrfToken == "" {
		_, csrfToken, _ = h.InitSession(username)
	}

	// Unit-price calculation
	config, _ := h.GetCreatorConfig(username, currency)
	traktirPrice := 1.0
	if tp, ok := config["TRAKTIR_PRICE"]; ok {
		if f, err := toFloat64(tp); err == nil && f > 0 {
			traktirPrice = f
		}
	}

	calculatedQty := int(math.Max(1, math.Round(amount/traktirPrice)))
	actualAmount := traktirPrice * float64(calculatedQty)

	form := url.Values{}
	form.Set("sb_token_csrf", csrfToken)
	form.Set("currency", currency)
	form.Set("amount", formatAmount(actualAmount))
	form.Set("qty", strconv.Itoa(calculatedQty))
	form.Set("support_duration", "30")
	form.Set("note", note)
	form.Set("fullname", fullname)
	form.Set("email", email)
	form.Set("is_agree", "1")
	form.Set("years18", "1")
	form.Set("is_vote", "0")
	form.Set("is_voice", "0")
	form.Set("is_mediashare", "0")
	form.Set("is_gif", "0")
	form.Set("is_sound", "0")
	form.Set("vote_id", "")
	form.Set("ms_maxtime", "")
	form.Set("start_from", "0")
	form.Set("ms_starthour", "0")
	form.Set("ms_startminute", "0")
	form.Set("ms_startsecond", "0")
	form.Set("spin_check", "0")
	form.Set("prev_url", h.url("/"+username+"/tribe?"))
	form.Set("hide_email", "0")
	form.Set("is_tiktok", "0")
	form.Set("tiktok_duration", "0")
	form.Set("is_instagram", "0")
	form.Set("instagram_duration", "0")
	form.Set("wishlist_id", "")
	form.Set("quickpay", "0")

	req, err := http.NewRequest(http.MethodPost, h.url("/"+username+"/donate/get-form-queue?buyme=1"), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", h.baseURL)
	req.Header.Set("Referer", h.url("/"+username+"/tribe?"))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")

	resp, err := h.do(req)
	if err != nil {
		return nil, fmt.Errorf("submit_donation: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("submit_donation: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result submitDonationResponse
	if err := parseJSON(string(body), &result); err != nil {
		return nil, fmt.Errorf("submit_donation: parse error: %w: body=%s", err, truncate(string(body), 200))
	}
	result.ActualAmount = actualAmount
	result.TraktirPrice = traktirPrice
	result.Qty = calculatedQty
	return &result, nil
}

// ── Step 2: payment settings ─────────────────────────────────────────

// GetPaymentSettings GETs /payment/pay/setting for available channels + fees.
func (h *HTTPClient) GetPaymentSettings(token string, amount float64, currency, country string) (*paymentSettingsResponse, error) {
	req, err := http.NewRequest(http.MethodGet, h.url("/payment/pay/setting"), nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("amount", formatAmount(amount))
	q.Set("currency", currency)
	q.Set("base_amount", formatAmount(amount))
	q.Set("base_currency", currency)
	q.Set("currency_def", currency)
	q.Set("convertion", currency)
	q.Set("country", country)
	q.Set("feature", "TRIBE")
	q.Set("is_borne_fee", "1")
	q.Set("risk", "")
	q.Set("message", "")
	q.Set("direct", "")
	q.Set("service_fee", "0")
	q.Set("token", token)
	q.Set("country_account", "")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Referer", h.url("/payment/x/"+token))

	resp, err := h.do(req)
	if err != nil {
		return nil, fmt.Errorf("get_payment_settings: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get_payment_settings: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var settings paymentSettingsResponse
	if err := parseJSON(string(body), &settings); err != nil {
		return nil, fmt.Errorf("get_payment_settings: parse error: %w: body=%s", err, truncate(string(body), 200))
	}
	return &settings, nil
}

// ── Step 2.5: visit payment page → extract CSRF ─────────────────────

var reCSRFToken = regexp.MustCompile(`name=["']sb_token_csrf["']\s+value=["']([^"']*)["']`)

// VisitPaymentPage GETs /payment/x/{token} to seed cookies and returns the CSRF token.
func (h *HTTPClient) VisitPaymentPage(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, h.url("/payment/x/"+token), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Referer", h.url("/payment/x/"+token))

	resp, err := h.do(req)
	if err != nil {
		return "", fmt.Errorf("visit_payment_page: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("visit_payment_page: HTTP %d", resp.StatusCode)
	}

	// Try extracting from HTML input field
	m := reCSRFToken.FindSubmatch(body)
	if m != nil {
		return string(m[1]), nil
	}

	// Fallback: read from cookie
	return h.getCSRFCookie(), nil
}

// ── Step 3: create payment ───────────────────────────────────────────

// CreatePayment POSTs to /payment/send/create and returns the payment URL/QR.
func (h *HTTPClient) CreatePayment(token, csrfToken, paymentMethod, typePayment, sourcePayment, currency, countryISO2, countryName string) (*createPaymentResponse, error) {
	bodyJSON := fmt.Sprintf(`{
		"sb_token_csrf":%q,"order_id":%q,"final_currency":%q,"currency_def":%q,
		"payment_method":%q,"type_payment":%q,"source_payment":%q,
		"country":%q,"country_pay":%q
	}`, csrfToken, token, currency, currency, paymentMethod, typePayment, sourcePayment, countryISO2, countryName)

	req, err := http.NewRequest(http.MethodPost, h.url("/payment/send/create"), strings.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Origin", h.baseURL)
	req.Header.Set("Referer", h.url("/payment/x/"+token))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")

	resp, err := h.do(req)
	if err != nil {
		return nil, fmt.Errorf("create_payment: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create_payment: HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result createPaymentResponse
	if err := parseJSON(string(body), &result); err != nil {
		return nil, fmt.Errorf("create_payment: parse error: %w: body=%s", err, truncate(string(body), 200))
	}
	return &result, nil
}

// ── Step 4: check payment status ─────────────────────────────────────

// CheckPaymentStatus GETs /payment/pending and parses HTML to detect status.
// Returns "paid", "expired", "failed", or "pending".
func (h *HTTPClient) CheckPaymentStatus(token, paymentMethod, invID string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, h.url("/payment/pending"), nil)
	if err != nil {
		return "", err
	}
	q := req.URL.Query()
	q.Set("type", paymentMethod)
	if invID == "" {
		invID = token
	}
	q.Set("inv_id", invID)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Referer", h.url("/payment/x/"+token))

	resp, err := h.do(req)
	if err != nil {
		return "", fmt.Errorf("check_payment_status: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check_payment_status: HTTP %d", resp.StatusCode)
	}

	htmlLower := strings.ToLower(string(body))
	switch {
	case strings.Contains(htmlLower, "payment successful"),
		strings.Contains(htmlLower, "payment success"),
		strings.Contains(htmlLower, "terima kasih"):
		return "paid", nil
	case strings.Contains(htmlLower, "expired") && strings.Contains(htmlLower, "payment expired"):
		return "expired", nil
	case strings.Contains(htmlLower, "payment failed"):
		return "failed", nil
	default:
		return "pending", nil
	}
}

// ── internal helpers ──────────────────────────────────────────────────

func (h *HTTPClient) getCSRFCookie() string {
	u, _ := url.Parse(h.baseURL)
	for _, c := range h.client.Jar.Cookies(u) {
		if c.Name == "sociabuzz_sb_cookie_csrf" || c.Name == "csrf_cookie_name" {
			return c.Value
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Close closes the underlying HTTP transport.
func (h *HTTPClient) Close() {
	h.client.CloseIdleConnections()
}