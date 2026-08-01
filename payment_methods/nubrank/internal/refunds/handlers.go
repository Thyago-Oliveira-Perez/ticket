package refunds

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"nubrank/internal/auth"
	nubrankjson "nubrank/internal/json"
	"nubrank/internal/payments"

	"github.com/go-chi/chi/v5"
)

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{service: service}
}

type createRefundRequest struct {
	AmountMinor int64 `json:"amount_minor,omitempty"`
}

func (h *handler) CreateRefund(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	paymentID := chi.URLParam(r, "id")

	var req createRefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	refund, err := h.service.CreateRefund(r.Context(), merchantID, paymentID, CreateRefundInput{AmountMinor: req.AmountMinor})
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, payments.ErrPaymentNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, ErrPaymentNotRefundable), errors.Is(err, ErrRefundExceedsRemaining):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	nubrankjson.Write(w, http.StatusCreated, refund)
}

func (h *handler) ListRefunds(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	paymentID := chi.URLParam(r, "id")

	refundList, err := h.service.ListRefunds(r.Context(), merchantID, paymentID)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, payments.ErrPaymentNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	nubrankjson.Write(w, http.StatusOK, refundList)
}
