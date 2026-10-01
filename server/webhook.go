package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// WebhookDispatcher polls pending payments and fires webhooks.
type WebhookDispatcher struct {
	store    *SQLiteSessionStore
	client   *http.Client
	polling  map[string]bool
	mu       sync.Mutex
	stopCh   chan struct{}
	stopped  chan struct{}
	cfg      *Settings
}

// NewWebhookDispatcher creates a webhook dispatcher.
func NewWebhookDispatcher(store *SQLiteSessionStore, cfg *Settings) *WebhookDispatcher {
	return &WebhookDispatcher{
		store:   store,
		client:  &http.Client{Timeout: 15 * time.Second},
		polling: make(map[string]bool),
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
		cfg:     cfg,
	}
}

// Start begins the background recovery loop.
func (d *WebhookDispatcher) Start() {
	if !d.cfg.WebhookEnabled {
		log.Println("[webhook] dispatcher disabled via WEBHOOK_ENABLED=false")
		return
	}
	go d.runLoop()
	log.Println("[webhook] dispatcher started")
}

// Stop signals the background loop to stop.
func (d *WebhookDispatcher) Stop() {
	close(d.stopCh)
	<-d.stopped
	d.client.CloseIdleConnections()
	log.Println("[webhook] dispatcher stopped")
}

func (d *WebhookDispatcher) runLoop() {
	defer close(d.stopped)

	// Resume pending payments after startup
	select {
	case <-time.After(3 * time.Second):
		d.resumePending()
	case <-d.stopCh:
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.resumePending()
		case <-d.stopCh:
			return
		}
	}
}

func (d *WebhookDispatcher) resumePending() {
	pending, err := d.store.ListPendingPayments()
	if err != nil {
		log.Printf("[webhook] resume error: %v", err)
		return
	}
	for _, p := range pending {
		invID := p.InvID
		d.mu.Lock()
		if d.polling[invID] {
			d.mu.Unlock()
			continue
		}
		d.polling[invID] = true
		d.mu.Unlock()

		go d.pollPayment(p.Token, invID, p.PaymentMethod)
	}
}

// Track registers a payment for background polling.
func (d *WebhookDispatcher) Track(token, invID, paymentMethod string) {
	if !d.cfg.WebhookEnabled {
		return
	}
	d.mu.Lock()
	if d.polling[invID] {
		d.mu.Unlock()
		return
	}
	d.polling[invID] = true
	d.mu.Unlock()

	go d.pollPayment(token, invID, paymentMethod)
}

func (d *WebhookDispatcher) pollPayment(token, invID, paymentMethod string) {
	defer func() {
		d.mu.Lock()
		delete(d.polling, invID)
		d.mu.Unlock()
	}()

	deadline := time.Now().Add(time.Duration(d.cfg.WebhookTimeout) * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-d.stopCh:
			return
		default:
		}

		donation, err := d.store.Get(token)
		if err != nil {
			log.Printf("[webhook] poll get session error inv_id=%s: %v", invID, err)
			time.Sleep(d.cfg.PollInterval())
			continue
		}

		status, err := donation.CheckPaymentStatus(context.Background(), paymentMethod, invID)
		if err != nil {
			log.Printf("[webhook] poll error inv_id=%s: %v", invID, err)
			status = "pending"
		}

		if isTerminal(status) {
			d.fireWebhook(token, invID, paymentMethod, status)
			return
		}

		time.Sleep(d.cfg.PollInterval())
	}

	// Timed out
	log.Printf("[webhook] poll timed out inv_id=%s", invID)
	d.fireWebhook(token, invID, paymentMethod, "expired")
}

func (d *WebhookDispatcher) fireWebhook(token, invID, paymentMethod, status string) {
	callbackURL := d.store.GetCallbackURL(token)
	if callbackURL == "" {
		d.store.MarkWebhookSent(invID, true)
		return
	}

	payload := map[string]any{
		"event":          "payment." + status,
		"token":          token,
		"inv_id":         invID,
		"payment_method": paymentMethod,
		"status":         status,
		"timestamp":      time.Now().Unix(),
	}

	body, _ := json.Marshal(payload)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if d.cfg.WebhookSecret != "" {
		headers["X-Webhook-Secret"] = d.cfg.WebhookSecret
		mac := hmac.New(sha256.New, []byte(d.cfg.WebhookSecret))
		mac.Write(body)
		headers["X-Webhook-Signature"] = hex.EncodeToString(mac.Sum(nil))
	}

	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequest("POST", callbackURL, strings.NewReader(string(body)))
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := d.client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			d.store.MarkWebhookSent(invID, true)
			log.Printf("[webhook] delivered to %s inv_id=%s status=%s", callbackURL, invID, status)
			return
		}
		if err != nil {
			log.Printf("[webhook] attempt %d to %s failed: %v", attempt+1, callbackURL, err)
		} else {
			resp.Body.Close()
			log.Printf("[webhook] attempt %d to %s returned HTTP %d", attempt+1, callbackURL, resp.StatusCode)
		}

		time.Sleep(2 * time.Second)
	}

	d.store.MarkWebhookSent(invID, false)
	log.Printf("[webhook] delivery failed after 3 attempts inv_id=%s", invID)
}

// PollInterval returns the webhook poll interval duration.
func (s *Settings) PollInterval() time.Duration {
	return time.Duration(s.WebhookPollInterval * float64(time.Second))
}

func isTerminal(status string) bool {
	return status == "paid" || status == "expired" || status == "failed"
}