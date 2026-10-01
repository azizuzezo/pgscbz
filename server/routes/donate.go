package routes

import (
	"encoding/json"
	"net/http"

	"github.com/hirotomasato/scbz/server"
	"github.com/hirotomasato/scbz/sociabuzz"
)

// HandleDonate handles POST /api/donate.
func HandleDonate(cfg *server.Settings, store server.SessionBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req server.DonateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server.JSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		username := req.Username
		if username == "" {
			username = cfg.DefaultCreator
		}
		if username == "" {
			server.JSONError(w, "No creator username supplied and DEFAULT_CREATOR is not set", http.StatusBadRequest)
			return
		}

		sb := sociabuzz.New(cfg.BaseURL)
		donation, err := sb.Donate(r.Context(), username, req.Amount, req.Country, req.Fullname, req.Email, req.Note)
		if err != nil {
			if _, ok := err.(*sociabuzz.UnsupportedCountryError); ok {
				server.JSONError(w, err.Error(), http.StatusBadRequest)
				return
			}
			if de, ok := err.(*sociabuzz.DonationError); ok && len(de.Errors) > 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]any{"message": de.Message, "errors": de.Errors})
				return
			}
			server.JSONError(w, err.Error(), http.StatusBadGateway)
			return
		}

		donation.CallbackURL = req.CallbackURL
		if err := store.Store(donation); err != nil {
			server.JSONError(w, "failed to store session", http.StatusInternalServerError)
			return
		}

		server.JSONOK(w, server.DonateResponse{
			Success:     true,
			Token:       donation.Token,
			Country:     donation.Country,
			Currency:    donation.Currency,
			Amount:      donation.Amount,
			RedirectURL: donation.RedirectURL,
			ExpiresIn:   donation.ExpiresIn,
		})
	}
}