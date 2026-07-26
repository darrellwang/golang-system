// Package event provides HTTP handlers, routes, middleware, and response helpers.
package event

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-commons/httputil"
	"github.com/darrell/golang-system/golang-app/internal/domain"
	"github.com/darrell/golang-system/golang-app/internal/service"
)

// ItemHandler is the HTTP entry-point adapter for items.
type ItemHandler struct {
	svc service.ItemService
	log zerolog.Logger
}

// NewItemHandler creates a new ItemHandler.
func NewItemHandler(svc service.ItemService, log zerolog.Logger) *ItemHandler {
	return &ItemHandler{svc: svc, log: log}
}

// Routes registers the item routes on the provided router.
func (h *ItemHandler) Routes(r chi.Router) {
	r.Route("/rest/v1/items", func(r chi.Router) {
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
	})
}

func (h *ItemHandler) create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateItemRequest
	// Limit request body to 1 MiB to mitigate oversized-body DoS attempts.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, err)
		return
	}
	item, err := h.svc.CreateItem(r.Context(), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.Created(w, item)
}

func (h *ItemHandler) get(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.GetItem(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, item)
}

func (h *ItemHandler) list(w http.ResponseWriter, r *http.Request) {
	offset := qInt(r, "offset", 0)
	limit := qInt(r, "limit", 20)
	status := r.URL.Query().Get("status")
	items, total, err := h.svc.ListItems(r.Context(), offset, limit, status)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, domain.Page{Items: items, Total: total, Offset: offset, Limit: limit})
}

func (h *ItemHandler) update(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateItemRequest
	// Limit request body to 1 MiB to mitigate oversized-body DoS attempts.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, err)
		return
	}
	item, err := h.svc.UpdateItem(r.Context(), chi.URLParam(r, "id"), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, item)
}

func (h *ItemHandler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteItem(r.Context(), chi.URLParam(r, "id")); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

func qInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}
