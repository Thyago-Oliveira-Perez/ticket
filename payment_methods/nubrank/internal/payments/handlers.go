package payments

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"nubrank/internal/auth"
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
	merchantID, _ := auth.MerchantID(r.Context())

	payments, err := h.service.ListPayments(r.Context(), merchantID)

	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusOK, payments)
}

func (h *handler) GetPayment(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	id := chi.URLParam(r, "id")

	payment, err := h.service.GetPayment(r.Context(), merchantID, id)
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
	CustomerID      string `json:"customer_id"`
	PaymentMethodID string `json:"payment_method_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	WebhookURL      string `json:"webhook_url,omitempty"`
}

func (h *handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())

	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	payment, err := h.service.CreatePayment(r.Context(), CreatePaymentInput{
		MerchantID:      merchantID,
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
		if errors.Is(err, ErrCustomerNotFound) || errors.Is(err, ErrPaymentMethodNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusCreated, payment)
}

type refundPaymentRequest struct {
	WebhookURL string `json:"webhook_url,omitempty"`
}

func (h *handler) RefundPayment(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	id := chi.URLParam(r, "id")

	var req refundPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	payment, err := h.service.RefundPayment(r.Context(), merchantID, id, RefundInput{WebhookURL: req.WebhookURL})
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrPaymentNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, ErrPaymentAlreadyRefunded), errors.Is(err, ErrPaymentNotApproved):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	nubrankjson.Write(w, http.StatusOK, payment)
}