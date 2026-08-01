package paymentmethods

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"nubrank/internal/auth"
	"nubrank/internal/customers"
	nubrankjson "nubrank/internal/json"

	"github.com/go-chi/chi/v5"
)

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{service: service}
}

type createPaymentMethodRequest struct {
	Number     string `json:"number"`
	ExpireYear int    `json:"expire_year"`
}

func (h *handler) CreatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	customerID := chi.URLParam(r, "customerId")

	var req createPaymentMethodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	pm, err := h.service.CreatePaymentMethod(r.Context(), merchantID, customerID, CreatePaymentMethodInput{
		Number:     req.Number,
		ExpireYear: req.ExpireYear,
	})
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, customers.ErrCustomerNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusCreated, pm)
}
