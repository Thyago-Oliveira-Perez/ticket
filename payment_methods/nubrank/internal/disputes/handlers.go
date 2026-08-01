package disputes

import (
	"errors"
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

func (h *handler) ListDisputes(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	paymentID := chi.URLParam(r, "id")

	disputeList, err := h.service.ListDisputes(r.Context(), merchantID, paymentID)
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

	nubrankjson.Write(w, http.StatusOK, disputeList)
}
