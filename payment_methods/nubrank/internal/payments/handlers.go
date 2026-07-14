package payments

import (
	"log"
	"net/http"
	"nubrank/internal/json"
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
	// 1. call repository
	// 2. return JSON in an HTTP response

	err := h.service.ListPayments(r.Context())

	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	payments := []string{"1", "2"}
	
	json.Write(w, http.StatusOK, payments)
}