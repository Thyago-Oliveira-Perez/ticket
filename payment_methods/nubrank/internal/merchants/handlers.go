package merchants

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	nubrankjson "nubrank/internal/json"
)

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{service: service}
}

type createMerchantRequest struct {
	Name string `json:"name"`
}

type createMerchantResponse struct {
	Merchant
	// APIKey is the plaintext key, shown only in this response. Callers
	// must store it themselves; nubrank never returns it again.
	APIKey string `json:"api_key"`
}

func (h *handler) CreateMerchant(w http.ResponseWriter, r *http.Request) {
	var req createMerchantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	merchant, apiKey, err := h.service.CreateMerchant(r.Context(), req.Name)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusCreated, createMerchantResponse{Merchant: merchant, APIKey: apiKey})
}
