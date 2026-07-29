package payments

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	nubrankjson "nubrank/internal/json"

	"github.com/go-chi/chi/v5"
)

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{
		service: service,
	}
}

func (h *handler) ListPayments(w http.ResponseWriter, r *http.Request) {
	payments, err := h.service.ListPayments(r.Context())

	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusOK, payments)
}

func (h *handler) GetPayment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	payment, err := h.service.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrPaymentNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusOK, payment)
}

type createPaymentRequest struct {
	MerchantID      string `json:"merchant_id"`
	CustomerID      string `json:"customer_id"`
	PaymentMethodID string `json:"payment_method_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	WebhookURL      string `json:"webhook_url,omitempty"`
}

func (h *handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	payment, err := h.service.CreatePayment(r.Context(), CreatePaymentInput{
		MerchantID:      req.MerchantID,
		CustomerID:      req.CustomerID,
		PaymentMethodID: req.PaymentMethodID,
		AmountMinor:     req.AmountMinor,
		Currency:        req.Currency,
		WebhookURL:      req.WebhookURL,
		IdempotencyKey:  r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusCreated, payment)
}