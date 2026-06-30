package event

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/darrell/golang-system/golang-commons/httputil"
	"github.com/darrell/golang-system/golang-app2/internal/domain"
	"github.com/darrell/golang-system/golang-app2/internal/service"
)

type OrderHandler struct {
	svc service.OrderService
	log zerolog.Logger
}

func NewOrderHandler(svc service.OrderService, log zerolog.Logger) *OrderHandler {
	return &OrderHandler{svc: svc, log: log}
}

func (h *OrderHandler) Routes(r chi.Router) {
	r.Route("/rest/v1/orders", func(r chi.Router) {
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.cancel)
	})
}

func (h *OrderHandler) create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, err)
		return
	}
	order, err := h.svc.CreateOrder(r.Context(), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.Created(w, order)
}

func (h *OrderHandler) get(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.GetOrder(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, order)
}

func (h *OrderHandler) list(w http.ResponseWriter, r *http.Request) {
	offset := qInt(r, "offset", 0)
	limit := qInt(r, "limit", 20)
	status := r.URL.Query().Get("status")
	orders, total, err := h.svc.ListOrders(r.Context(), offset, limit, status)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]any{
		"orders": orders, "total": total, "offset": offset, "limit": limit,
	})
}

func (h *OrderHandler) update(w http.ResponseWriter, r *http.Request) {
	var req domain.UpdateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.Error(w, err)
		return
	}
	order, err := h.svc.UpdateOrder(r.Context(), chi.URLParam(r, "id"), &req)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, order)
}

func (h *OrderHandler) cancel(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.CancelOrder(r.Context(), chi.URLParam(r, "id")); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.NoContent(w)
}

func qInt(r *http.Request, k string, d int) int {
	if v := r.URL.Query().Get(k); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return d
}
