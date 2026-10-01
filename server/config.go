// Package server provides configuration, auth, session store, routes, and webhook
// for the SociaBuzz payment gateway HTTP server.
package server

import (
	"os"
	"strconv"
	"strings"
)

// Settings holds all runtime configuration from environment variables.
type Settings struct {
	// SociaBuzz backend
	BaseURL        string
	DefaultCreator string

	// Gateway server
	Host string
	Port int

	// Authentication
	APIKey  string
	APIKeys []string

	// Session persistence
	SessionBackend string
	SessionDBPath  string

	// Webhook / callback
	WebhookEnabled      bool
	WebhookPollInterval float64
	WebhookTimeout      float64
	WebhookSecret       string
}

// AllKeys returns the union of APIKey and APIKeys.
func (s *Settings) AllKeys() map[string]bool {
	keys := make(map[string]bool)
	if s.APIKey != "" {
		keys[s.APIKey] = true
	}
	for _, k := range s.APIKeys {
		if k != "" {
			keys[k] = true
		}
	}
	return keys
}

// AuthEnabled returns true if at least one API key is configured.
func (s *Settings) AuthEnabled() bool {
	return s.APIKey != "" || len(s.APIKeys) > 0
}

// LoadSettings reads configuration from environment variables.
func LoadSettings() *Settings {
	return &Settings{
		BaseURL:             envStr("SOCIABUZZ_BASE_URL", "https://sociabuzz.com"),
		DefaultCreator:      firstEnv("SOCIABUZZ_USERNAME", "DEFAULT_CREATOR"),
		Host:                envStr("API_HOST", "0.0.0.0"),
		Port:                envInt("API_PORT", 8000),
		APIKey:              os.Getenv("API_KEY"),
		APIKeys:             splitEnv("API_KEYS"),
		SessionBackend:      strings.ToLower(envStr("SESSION_BACKEND", "sqlite")),
		SessionDBPath:       envStr("SESSION_DB_PATH", "scbz_sessions.db"),
		WebhookEnabled:      envBool("WEBHOOK_ENABLED", true),
		WebhookPollInterval: envFloat("WEBHOOK_POLL_INTERVAL", 5),
		WebhookTimeout:      envFloat("WEBHOOK_TIMEOUT", 3600),
		WebhookSecret:       os.Getenv("WEBHOOK_SECRET"),
	}
}

func envStr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(name string, def float64) float64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envBool(name string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func splitEnv(name string) []string {
	v := os.Getenv(name)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}