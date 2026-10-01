package routes

import (
	"encoding/json"
	"net/http"

	"github.com/hirotomasato/scbz/server"
)

// HandlePaymentMethods handles POST /api/payment/methods.
func HandlePaymentMethods(cfg *server.Settings, store server.SessionBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req server.PaymentMethodsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server.JSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		donation, err := store.Get(req.Token)
		if err != nil {
			server.JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		methods, err := donation.GetPaymentMethods(r.Context())
		if err != nil {
			server.JSONError(w, "failed to fetch payment methods: "+err.Error(), http.StatusBadGateway)
			return
		}

		// Persist CSRF + cookies back (if sqlite store via update-csrf pattern)
		if ss, ok := store.(*server.SQLiteSessionStore); ok {
			ss.UpdateCSRF(req.Token, donation.GetCSRF(), donation.HTTPClient().SerializeCookies())
		}

		items := make([]server.PaymentMethodItem, 0, len(methods))
		for _, m := range methods {
			items = append(items, server.PaymentMethodItem{
				ID:      m.ID,
				Name:    m.Name,
				Label:   m.Label,
				Group:   m.Group,
				Gateway: m.Gateway,
				Fee:     toSchemaFee(m.Fee),
			})
		}

		server.JSONOK(w, server.PaymentMethodsResponse{
			Success:  true,
			Token:    req.Token,
			Country:  donation.Country,
			Currency: donation.Currency,
			Amount:   donation.Amount,
			Methods:  items,
			Total:    len(items),
		})
	}
}

// HandleCreatePayment handles POST /api/payment/create.
func HandleCreatePayment(cfg *server.Settings, store server.SessionBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req server.CreatePaymentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server.JSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		donation, err := store.Get(req.Token)
		if err != nil {
			server.JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		payment, err := donation.CreatePayment(r.Context(), req.PaymentMethod)
		if err != nil {
			server.JSONError(w, err.Error(), http.StatusNotFound)
			return
		}

		// Persist CSRF/cookies + payment record
		if ss, ok := store.(*server.SQLiteSessionStore); ok {
			ss.UpdateCSRF(req.Token, donation.GetCSRF(), donation.HTTPClient().SerializeCookies())
			feeMap := map[string]any{
				"fee_type":   payment.Fee.FeeType,
				"fee_rate":   payment.Fee.FeeRate,
				"fee_fixed":  payment.Fee.FeeFixed,
				"total_fee":  payment.Fee.TotalFee,
				"total_pay":  payment.Fee.TotalPay,
				"min_amount": payment.Fee.MinAmount,
				"max_amount": payment.Fee.MaxAmount,
			}
			ss.StorePayment(req.Token, payment.InvID, payment.Method, payment.Label,
				payment.Gateway, strPtr(payment.PaymentURL), strPtr(payment.QRString), feeMap)
		}

		resp := server.CreatePaymentResponse{
			Success:       true,
			PaymentMethod: payment.Method,
			Label:         payment.Label,
			Gateway:       payment.Gateway,
			Fee:           toSchemaFee(payment.Fee),
			PaymentURL:    strPtr(payment.PaymentURL),
			QRString:      strPtr(payment.QRString),
			Token:         payment.Token,
			InvID:         strPtr(payment.InvID),
		}
		server.JSONOK(w, resp)
	}
}

// HandleCheckStatus handles POST /api/payment/status.
func HandleCheckStatus(cfg *server.Settings, store server.SessionBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req server.PaymentStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			server.JSONError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		donation, err := store.Get(req.Token)
		if err != nil {
			server.JSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		status, err := donation.CheckPaymentStatus(r.Context(), req.PaymentMethod, req.InvID)
		if err != nil {
			server.JSONError(w, "failed to check status: "+err.Error(), http.StatusBadGateway)
			return
		}

		server.JSONOK(w, server.PaymentStatusResponse{
			Success: true,
			Token:   req.Token,
			Status:  status,
		})
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}