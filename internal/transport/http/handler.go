package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/alkmc/storefront/internal/auth"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type (
	processor interface {
		Create(context.Context, domain.Product) (domain.Product, error)
		FindByID(context.Context, uuid.UUID) (domain.Product, error)
		FindAll(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
		Update(context.Context, domain.Product) (domain.Product, error)
		Delete(context.Context, uuid.UUID) error
		Purchase(context.Context, domain.UserID, uuid.UUID, int64) (domain.Product, domain.Order, error)
		FindOrder(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
		FindOrders(context.Context, domain.UserID, uuid.NullUUID, int) (domain.OrderPage, error)
	}
	Handler struct {
		logger         *slog.Logger
		processor      processor
		requestTimeout time.Duration
	}
	moneyInput struct {
		MinorAmount int64           `json:"minorAmount"`
		Currency    domain.Currency `json:"currency"`
	}
	addInput struct {
		Name  string     `json:"name"`
		Stock int64      `json:"stock"`
		Price moneyInput `json:"price"`
	}
	updateInput struct {
		Name  string     `json:"name"`
		Price moneyInput `json:"price"`
	}
	purchaseInput struct {
		Quantity int64 `json:"quantity"`
	}
)

// NewHandler initializes a product API handler with its required dependencies
func NewHandler(p processor, requestTimeout time.Duration, l *slog.Logger) *Handler {
	return &Handler{
		logger:         l,
		processor:      p,
		requestTimeout: requestTimeout,
	}
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	p, err := h.processor.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			respondError(w, http.StatusNotFound, "product not found")
			return
		}
		h.respondServerError(
			w, err, "failed to find product by id",
			slog.Any("error", err), slog.String("id", id.String()),
		)
		return
	}
	respond(w, http.StatusOK, toProductResponse(p))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := parseLimit(q.Get("limit"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	cursor, err := domain.ParseCursor(q.Get("cursor"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	page, err := h.processor.FindAll(ctx, cursor, limit)
	if err != nil {
		h.respondServerError(w, err, "failed to find all products", slog.Any("error", err))
		return
	}
	respond(w, http.StatusOK, toProductsPage(page))
}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength == 0 {
		respondError(w, http.StatusBadRequest, msgEmptyBody)
		return
	}

	var in addInput
	if err := decodeBody(r.Body, &in); err != nil {
		h.logger.Warn("decode body failed", slog.Any("error", err))
		respondDecodeError(w, err)
		return
	}

	p := domain.Product{Name: in.Name, Price: toMoney(in.Price), Stock: in.Stock}
	if err := p.Validate(); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	result, err := h.processor.Create(ctx, p)
	if err != nil {
		h.respondServerError(w, err, "failed to create product", slog.Any("error", err))
		return
	}
	respond(w, http.StatusCreated, toProductResponse(result))
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	if err := h.processor.Delete(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			respondError(w, http.StatusNotFound, "unable to delete product, which does not exist")
			return
		}
		if errors.Is(err, domain.ErrProductInUse) {
			respondError(w, http.StatusConflict, "product has existing orders")
			return
		}
		h.respondServerError(
			w, err, "failed to delete product",
			slog.Any("error", err), slog.String("id", id.String()),
		)
		return
	}
	respond(w, http.StatusOK, messageResponse{Message: "product deleted"})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if r.ContentLength == 0 {
		respondError(w, http.StatusBadRequest, msgEmptyBody)
		return
	}

	var in updateInput
	if err := decodeBody(r.Body, &in); err != nil {
		h.logger.Warn("decode body failed", slog.Any("error", err))
		respondDecodeError(w, err)
		return
	}

	p := domain.Product{ID: id, Name: in.Name, Price: toMoney(in.Price)}
	if err := p.Validate(); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	updated, err := h.processor.Update(ctx, p)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			respondError(w, http.StatusNotFound, "unable to update product, which does not exist")
			return
		}
		h.respondServerError(
			w, err, "failed to update product",
			slog.Any("error", err), slog.String("id", id.String()),
		)
		return
	}
	respond(w, http.StatusOK, toProductResponse(updated))
}

func (h *Handler) Purchase(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if r.ContentLength == 0 {
		respondError(w, http.StatusBadRequest, msgEmptyBody)
		return
	}

	var in purchaseInput
	if err := decodeBody(r.Body, &in); err != nil {
		h.logger.Warn("decode body failed", slog.Any("error", err))
		respondDecodeError(w, err)
		return
	}
	if !domain.ValidPurchaseQuantity(in.Quantity) {
		respondError(w, http.StatusUnprocessableEntity, msgInvalidQuantity)
		return
	}

	userID, ok := h.callerID(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	p, o, err := h.processor.Purchase(ctx, userID, id, in.Quantity)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			respondError(w, http.StatusNotFound, "product not found")
		case errors.Is(err, domain.ErrInsufficientStock):
			respondError(w, http.StatusConflict, "insufficient stock")
		default:
			h.respondServerError(
				w, err, "failed to purchase product",
				slog.Any("error", err), slog.String("id", id.String()),
			)
		}
		return
	}
	respond(w, http.StatusOK, toPurchaseResponse(p, o))
}

// respondServerError maps infrastructure failures to 503 or 500 and logs them.
func (h *Handler) respondServerError(w http.ResponseWriter, err error, logMsg string, attrs ...any) {
	if errors.Is(err, domain.ErrUnavailable) {
		h.logger.Warn(logMsg, attrs...)
		respondError(w, http.StatusServiceUnavailable, msgUnavailable)
		return
	}
	h.internalError(w, logMsg, attrs...)
}

// internalError logs the failure with attrs and replies with a generic 500.
func (h *Handler) internalError(w http.ResponseWriter, msg string, attrs ...any) {
	h.logger.Error(msg, attrs...)
	respondError(w, http.StatusInternalServerError, msgInternalError)
}

// callerID reads the authenticated user, its absence on a protected route is a mux wiring bug.
func (h *Handler) callerID(w http.ResponseWriter, r *http.Request) (domain.UserID, bool) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		h.internalError(w, "user id missing on protected route")
	}
	return domain.UserID(userID), ok
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return domain.DefaultPageSize, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: %q", raw)
	}
	return domain.NormalizePageSize(n), nil
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	o, err := h.processor.FindOrder(ctx, userID, domain.OrderID(id))
	if err != nil {
		// a foreign order takes the same path, the caller cannot tell it from a missing one
		if errors.Is(err, domain.ErrNotFound) {
			respondError(w, http.StatusNotFound, "order not found")
			return
		}
		h.respondServerError(
			w, err, "failed to find order",
			slog.Any("error", err), slog.String("id", id.String()),
		)
		return
	}
	respond(w, http.StatusOK, toOrderResponse(o))
}

func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.callerID(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, err := parseLimit(q.Get("limit"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	cursor, err := domain.ParseCursor(q.Get("cursor"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	page, err := h.processor.FindOrders(ctx, userID, cursor, limit)
	if err != nil {
		h.respondServerError(w, err, "failed to find orders", slog.Any("error", err))
		return
	}
	respond(w, http.StatusOK, toOrdersPage(page))
}
