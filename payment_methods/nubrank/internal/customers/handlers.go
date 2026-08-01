package customers

import (
	"encoding/json"
	"errors"
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

type createCustomerRequest struct {
	Email string `json:"email"`
}

func (h *handler) CreateCustomer(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())

	var req createCustomerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	customer, err := h.service.CreateCustomer(r.Context(), merchantID, req.Email)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusCreated, customer)
}

func (h *handler) GetCustomer(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())
	id := chi.URLParam(r, "id")

	customer, err := h.service.GetCustomer(r.Context(), merchantID, id)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, ErrCustomerNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusOK, customer)
}
