package payouts

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
	return &handler{service: service}
}

type createPayoutRequest struct {
	AmountMinor int64  `json:"amount_minor,omitempty"`
	Currency    string `json:"currency"`
}

func (h *handler) CreatePayout(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())

	var req createPayoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	payout, err := h.service.CreatePayout(r.Context(), merchantID, CreatePayoutInput{
		AmountMinor: req.AmountMinor,
		Currency:    req.Currency,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrInsufficientBalance):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	nubrankjson.Write(w, http.StatusCreated, payout)
}

func (h *handler) GetPayout(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	id := chi.URLParam(r, "id")

	payout, err := h.service.GetPayout(r.Context(), merchantID, id)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrPayoutNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusOK, payout)
}

func (h *handler) ListPayouts(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())

	payoutList, err := h.service.ListPayouts(r.Context(), merchantID)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusOK, payoutList)
}
