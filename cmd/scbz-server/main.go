package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/hirotomasato/scbz/server"
	"github.com/hirotomasato/scbz/server/routes"
)

func main() {
	cfg := server.LoadSettings()

	if !cfg.AuthEnabled() {
		// Anyone who can reach the port could open payment sessions on the creator's account.
		if os.Getenv("ALLOW_NO_AUTH") != "true" {
			log.Fatal("API_KEY is not set. Set API_KEY (or API_KEYS), or ALLOW_NO_AUTH=true for local testing only.")
		}
		log.Println("WARNING: API authentication DISABLED (ALLOW_NO_AUTH=true)")
	} else {
		log.Printf("API authentication ENABLED (%d key(s))", len(cfg.AllKeys()))
	}

	// Session store
	store, err := server.NewSessionStore(cfg)
	if err != nil {
		log.Fatalf("Failed to create session store: %v", err)
	}
	log.Printf("Session backend: %s", cfg.SessionBackend)

	// Webhook dispatcher (SQLite only)
	var webhook *server.WebhookDispatcher
	if ss, ok := store.(*server.SQLiteSessionStore); ok && cfg.WebhookEnabled {
		webhook = server.NewWebhookDispatcher(ss, cfg)
		webhook.Start()
		defer webhook.Stop()
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(chimw.AllowContentType("application/json"))
	r.Use(server.APIAuthMiddleware(cfg))

	// Health
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":          "ok",
			"active_sessions": store.Count(),
			"auth_enabled":    cfg.AuthEnabled(),
			"webhook_enabled": cfg.WebhookEnabled,
		})
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Step 0 + 1
		r.Post("/donate", routes.HandleDonate(cfg, store))

		// Fees
		r.Post("/fees", routes.HandleGetFees(cfg, store))
		r.Get("/fees/countries", routes.HandleListCountries(cfg))

		// Payment steps 2-4
		r.Route("/payment", func(r chi.Router) {
			r.Post("/methods", routes.HandlePaymentMethods(cfg, store))
			r.Post("/create", routes.HandleCreatePayment(cfg, store))
			r.Post("/status", routes.HandleCheckStatus(cfg, store))
		})
	})

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	httpServer := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-sigCh
	log.Println("Shutting down...")

	store.CloseAll()
	httpServer.Close()
}