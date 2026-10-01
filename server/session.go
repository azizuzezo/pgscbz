package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hirotomasato/scbz/sociabuzz"

	_ "modernc.org/sqlite"
)

// SessionBackend is the interface for session persistence.
type SessionBackend interface {
	Get(token string) (*sociabuzz.Donation, error)
	Store(donation *sociabuzz.Donation) error
	Delete(token string) error
	CloseAll() error
	Count() int
}

// ── Memory backend ───────────────────────────────────────────────────

type MemorySessionStore struct {
	mu        sync.RWMutex
	sessions  map[string]*sociabuzz.Donation
}

func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{sessions: make(map[string]*sociabuzz.Donation)}
}

func (m *MemorySessionStore) Get(token string) (*sociabuzz.Donation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.sessions[token]
	if !ok {
		return nil, fmt.Errorf("session not found for token '%s'", token)
	}
	return d, nil
}

func (m *MemorySessionStore) Store(d *sociabuzz.Donation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[d.Token] = d
	return nil
}

func (m *MemorySessionStore) Delete(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
	return nil
}

func (m *MemorySessionStore) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.sessions {
		d.Close()
	}
	m.sessions = make(map[string]*sociabuzz.Donation)
	return nil
}

func (m *MemorySessionStore) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// ── SQLite backend ───────────────────────────────────────────────────

const sessionSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    token        TEXT PRIMARY KEY,
    country      TEXT NOT NULL DEFAULT '',
    currency     TEXT NOT NULL DEFAULT '',
    amount       REAL NOT NULL DEFAULT 0,
    redirect_url TEXT NOT NULL DEFAULT '',
    expires_in   INTEGER NOT NULL DEFAULT 3600,
    csrf         TEXT NOT NULL DEFAULT '',
    callback_url TEXT NOT NULL DEFAULT '',
    cookies      TEXT NOT NULL DEFAULT '{}',
    created_at   REAL NOT NULL,
    updated_at   REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS payments (
    inv_id         TEXT PRIMARY KEY,
    token          TEXT NOT NULL,
    payment_method TEXT NOT NULL DEFAULT '',
    label          TEXT NOT NULL DEFAULT '',
    gateway        TEXT NOT NULL DEFAULT '',
    payment_url    TEXT,
    qr_string      TEXT,
    fee_json       TEXT NOT NULL DEFAULT '{}',
    webhook_sent   INTEGER NOT NULL DEFAULT 0,
    webhook_attempts INTEGER NOT NULL DEFAULT 0,
    created_at     REAL NOT NULL,
    FOREIGN KEY (token) REFERENCES sessions(token)
);
CREATE INDEX IF NOT EXISTS idx_payments_token ON payments(token);
`

type SQLiteSessionStore struct {
	db     *sql.DB
	mu     sync.Mutex
	dbPath string
}

func NewSQLiteSessionStore(dbPath string) (*SQLiteSessionStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite single-writer
	store := &SQLiteSessionStore{db: db, dbPath: dbPath}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteSessionStore) init() error {
	_, err := s.db.Exec(sessionSchema)
	return err
}

func (s *SQLiteSessionStore) Get(token string) (*sociabuzz.Donation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow("SELECT * FROM sessions WHERE token = ?", token)
	var (
		t, country, currency, redirectURL, csrf, callbackURL, cookiesJSON string
		amount                                                             float64
		expiresIn                                                          int
		createdAt, updatedAt                                               float64
	)
	err := row.Scan(&t, &country, &currency, &amount, &redirectURL, &expiresIn,
		&csrf, &callbackURL, &cookiesJSON, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found for token '%s'", token)
	}
	if err != nil {
		return nil, fmt.Errorf("session fetch: %w", err)
	}

	httpClient := sociabuzz.NewHTTPClient("https://sociabuzz.com")
	donation := &sociabuzz.Donation{
		Token:       t,
		Country:     country,
		Currency:    currency,
		Amount:      amount,
		RedirectURL: redirectURL,
		ExpiresIn:   expiresIn,
	}

	// Restore CSRF
	donation.SetCSRF(csrf)

	// Restore cookies
	var cookies map[string]string
	if err := json.Unmarshal([]byte(cookiesJSON), &cookies); err == nil {
		httpClient.RestoreCookies(cookies)
	}

	return donation, nil
}

func (s *SQLiteSessionStore) Store(d *sociabuzz.Donation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := d.Serialize()
	now := float64(time.Now().Unix())
	cookiesJSON, _ := json.Marshal(data["cookies"])

	_, err := s.db.Exec(`
		INSERT INTO sessions
			(token, country, currency, amount, redirect_url, expires_in,
			 csrf, callback_url, cookies, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(token) DO UPDATE SET
			csrf = excluded.csrf,
			callback_url = excluded.callback_url,
			cookies = excluded.cookies,
			updated_at = excluded.updated_at
	`, d.Token, d.Country, d.Currency, d.Amount, d.RedirectURL, d.ExpiresIn,
		data["csrf"], data["callback_url"], string(cookiesJSON), now, now)
	return err
}

func (s *SQLiteSessionStore) UpdateCSRF(token, csrf string, cookies map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cookiesJSON, _ := json.Marshal(cookies)
	_, err := s.db.Exec(
		"UPDATE sessions SET csrf = ?, cookies = ?, updated_at = ? WHERE token = ?",
		csrf, string(cookiesJSON), float64(time.Now().Unix()), token)
	return err
}

func (s *SQLiteSessionStore) GetCallbackURL(token string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var url string
	s.db.QueryRow("SELECT callback_url FROM sessions WHERE token = ?", token).Scan(&url)
	return url
}

func (s *SQLiteSessionStore) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	s.db.Exec("DELETE FROM payments WHERE token = ?", token)
	return nil
}

func (s *SQLiteSessionStore) StorePayment(token, invID, paymentMethod, label, gateway string, paymentURL, qrString *string, fee map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	feeJSON, _ := json.Marshal(fee)
	now := float64(time.Now().Unix())

	_, err := s.db.Exec(`
		INSERT INTO payments
			(inv_id, token, payment_method, label, gateway,
			 payment_url, qr_string, fee_json, webhook_sent,
			 webhook_attempts, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?)
		ON CONFLICT(inv_id) DO UPDATE SET
			payment_url = excluded.payment_url,
			qr_string = excluded.qr_string
	`, invID, token, paymentMethod, label, gateway, paymentURL, qrString, string(feeJSON), now)
	return err
}

// PendingPayment represents a payment pending webhook delivery.
type PendingPayment struct {
	InvID         string
	Token         string
	PaymentMethod string
	Label         string
	Gateway       string
	PaymentURL    *string
	QRString      *string
	FeeJSON       string
	WebhookSent   int
}

func (s *SQLiteSessionStore) ListPendingPayments() ([]PendingPayment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT * FROM payments WHERE webhook_sent = 0")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []PendingPayment
	for rows.Next() {
		var p PendingPayment
		var createdAt float64
		err := rows.Scan(&p.InvID, &p.Token, &p.PaymentMethod, &p.Label, &p.Gateway,
			&p.PaymentURL, &p.QRString, &p.FeeJSON, &p.WebhookSent, &createdAt)
		if err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	return payments, nil
}

func (s *SQLiteSessionStore) MarkWebhookSent(invID string, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if success {
		_, err := s.db.Exec("UPDATE payments SET webhook_sent = 1 WHERE inv_id = ?", invID)
		return err
	}
	_, err := s.db.Exec("UPDATE payments SET webhook_attempts = webhook_attempts + 1 WHERE inv_id = ?", invID)
	return err
}

func (s *SQLiteSessionStore) CloseAll() error {
	return s.db.Close()
}

func (s *SQLiteSessionStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	return count
}

// ── Factory ──────────────────────────────────────────────────────────

// NewSessionStore creates the appropriate store based on settings.
func NewSessionStore(settings *Settings) (SessionBackend, error) {
	if settings.SessionBackend == "memory" {
		return NewMemorySessionStore(), nil
	}
	return NewSQLiteSessionStore(settings.SessionDBPath)
}