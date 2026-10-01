package routes

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/hirotomasato/scbz/server"
	"github.com/hirotomasato/scbz/sociabuzz"
)

// HandleGetFees handles POST /api/fees.
func HandleGetFees(cfg *server.Settings, store server.SessionBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req server.FeesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server.JSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		info, ok := sociabuzz.SupportedCountry(req.Country)
		if !ok {
			server.JSONError(w, "unsupported country '"+req.Country+"'", http.StatusBadRequest)
			return
		}

		username := req.Username
		if username == "" {
			username = cfg.DefaultCreator
		}
		if username == "" {
			server.JSONError(w, "No creator username supplied", http.StatusBadRequest)
			return
		}

		calc := sociabuzz.NewFeeCalculator(cfg.BaseURL, username)
		estimates, err := calc.Estimate(r.Context(), req.Country, req.Amount, username)
		calc.Close()
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

		methods := make([]server.FeeMethodItem, 0, len(estimates))
		for _, e := range estimates {
			methods = append(methods, server.FeeMethodItem{
				ID:       e.ID,
				Name:     e.Name,
				Label:    e.Label,
				Group:    e.Group,
				Gateway:  e.Gateway,
				Fee:      toSchemaFee(e.Fee),
				Currency: e.Currency,
				Amount:   e.Amount,
			})
		}

		server.JSONOK(w, server.FeesResponse{
			Success:  true,
			Country:  req.Country,
			Currency: info.Currency,
			Amount:   req.Amount,
			Methods:  methods,
			Total:    len(methods),
		})
	}
}

// HandleListCountries handles GET /api/fees/countries.
func HandleListCountries(cfg *server.Settings) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		countries := sociabuzz.AllCountries()
		sort.Slice(countries, func(i, j int) bool {
			return countries[i].Name < countries[j].Name
		})

		items := make([]server.CountriesItem, 0, len(countries))
		for _, c := range countries {
			items = append(items, server.CountriesItem{
				Country:  c.Name,
				Currency: c.Currency,
				ISO2:     c.ISO2,
			})
		}

		server.JSONOK(w, server.CountriesResponse{
			Success:   true,
			Total:     len(items),
			Countries: items,
		})
	}
}

func toSchemaFee(fee sociabuzz.FeeInfo) server.SociaFeeInfo {
	return server.SociaFeeInfo{
		FeeType:   fee.FeeType,
		FeeRate:   fee.FeeRate,
		FeeFixed:  fee.FeeFixed,
		TotalFee:  fee.TotalFee,
		TotalPay:  fee.TotalPay,
		MinAmount: fee.MinAmount,
		MaxAmount: fee.MaxAmount,
	}
}