package webhookendpoints

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"nubrank/internal/auth"
	nubrankjson "nubrank/internal/json"
)

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{service: service}
}

type createEndpointRequest struct {
	URL string `json:"url"`
}

type createEndpointResponse struct {
	Endpoint
	// Secret is shown only in this response; Endpoint.Secret is otherwise
	// excluded from JSON.
	Secret string `json:"secret"`
}

func (h *handler) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	merchantID, _ := auth.MerchantID(r.Context())

	var req createEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	endpoint, err := h.service.CreateEndpoint(r.Context(), merchantID, req.URL)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	nubrankjson.Write(w, http.StatusCreated, createEndpointResponse{Endpoint: endpoint, Secret: endpoint.Secret})
}
