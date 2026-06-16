package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/alkmc/storefront/internal/entity"
	"github.com/google/uuid"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

type (
	processor interface {
		Create(context.Context, entity.Product) (entity.Product, error)
		FindByID(context.Context, uuid.UUID) (entity.Product, error)
		FindAll(context.Context, uuid.NullUUID, int) (entity.ProductPage, error)
		Update(context.Context, entity.Product) error
		Delete(context.Context, uuid.UUID) error
	}

	Handler struct {
		logger         *slog.Logger
		processor      processor
		requestTimeout time.Duration
	}

	moneyInput struct {
		MinorAmount int64           `json:"minorAmount"`
		Currency    entity.Currency `json:"currency"`
	}

	productInput struct {
		Name  string     `json:"name"`
		Price moneyInput `json:"price"`
	}
)

// NewHandler initializes a product API handler with its required dependencies
func NewHandler(l *slog.Logger, p processor, requestTimeout time.Duration) *Handler {
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
		if errors.Is(err, entity.ErrNotFound) {
			respondError(w, http.StatusNotFound, "product not found")
			return
		}
		h.internalError(
			w, "failed to find product by id",
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
	cursor, err := parseCursor(q.Get("cursor"))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	page, err := h.processor.FindAll(ctx, cursor, limit)
	if err != nil {
		h.internalError(w, "failed to find all products", slog.Any("error", err))
		return
	}
	respond(w, http.StatusOK, toProductsPage(page))
}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength == 0 {
		respondError(w, http.StatusBadRequest, msgEmptyBody)
		return
	}

	var in productInput
	if err := decodeBody(r.Body, &in); err != nil {
		h.logger.Warn("decode body failed", slog.Any("error", err))
		respondDecodeError(w, err)
		return
	}

	p := entity.Product{Name: in.Name, Price: toMoney(in.Price)}
	if err := p.Validate(); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	result, err := h.processor.Create(ctx, p)
	if err != nil {
		h.internalError(w, "failed to create product", slog.Any("error", err))
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
		if errors.Is(err, entity.ErrNotFound) {
			respondError(w, http.StatusNotFound, "unable to delete product, which does not exist")
			return
		}
		h.internalError(
			w, "failed to delete product",
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

	var in productInput
	if err := decodeBody(r.Body, &in); err != nil {
		h.logger.Warn("decode body failed", slog.Any("error", err))
		respondDecodeError(w, err)
		return
	}

	p := entity.Product{ID: id, Name: in.Name, Price: toMoney(in.Price)}
	if err := p.Validate(); err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	if err := h.processor.Update(ctx, p); err != nil {
		if errors.Is(err, entity.ErrNotFound) {
			respondError(w, http.StatusNotFound, "unable to update product, which does not exist")
			return
		}
		h.internalError(w, "failed to update product",
			slog.Any("error", err), slog.String("id", id.String()))
		return
	}
	respond(w, http.StatusOK, toProductResponse(p))
}

// internalError logs the failure with attrs and replies with a generic 500.
func (h *Handler) internalError(w http.ResponseWriter, msg string, attrs ...any) {
	h.logger.Error(msg, attrs...)
	respondError(w, http.StatusInternalServerError, msgInternalError)
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: %q", raw)
	}
	if n <= 0 {
		return defaultLimit, nil
	}
	return min(n, maxLimit), nil
}

func parseCursor(raw string) (uuid.NullUUID, error) {
	if raw == "" {
		return uuid.NullUUID{}, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.NullUUID{}, fmt.Errorf("invalid cursor: %q", raw)
	}
	return uuid.NullUUID{UUID: id, Valid: true}, nil
}
