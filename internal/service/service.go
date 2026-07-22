package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

type (
	productStorer interface {
		Save(context.Context, domain.Product) (domain.Product, error)
		FindByID(context.Context, uuid.UUID) (domain.Product, error)
		FindAll(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
		Update(context.Context, domain.Product) (domain.Product, error)
		Delete(context.Context, uuid.UUID) error
	}
	orderStorer interface {
		// CreateOrder records an order and decrements the product stock in one tx.
		CreateOrder(context.Context, domain.Order) (domain.Product, domain.Order, error)
		FindOrder(context.Context, domain.UserID, domain.OrderID) (domain.Order, error)
		FindOrders(context.Context, domain.UserID, uuid.NullUUID, int) (domain.OrderPage, error)
	}
	storer interface {
		productStorer
		orderStorer
	}
	cacher interface {
		Set(context.Context, string, domain.Product, cache.Entry) error
		SetMissing(context.Context, string, cache.Entry) error
		Get(context.Context, string) (cache.Entry, error)
		Invalidate(context.Context, string) error
	}
	Service struct {
		logger      *slog.Logger
		store       storer
		cache       cacher
		loads       callGroup[domain.Product]
		loadTimeout time.Duration
	}
)

// NewService applies loadTimeout to every cache and store call that runs detached from
// the caller, so a client disconnect cannot leave the cache unwritten.
func NewService(
	st storer, c cacher, loadTimeout time.Duration, l *slog.Logger,
) *Service {
	return new(Service{
		logger:      l,
		store:       st,
		cache:       c,
		loads:       callGroup[domain.Product]{timeout: loadTimeout},
		loadTimeout: loadTimeout,
	})
}

func (s *Service) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Product{}, fmt.Errorf("failed to generate uuid: %w", err)
	}
	p.ID = id

	return s.store.Save(ctx, p)
}

func (s *Service) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	key := id.String()
	entry, err := s.cache.Get(ctx, key)
	if err != nil {
		s.logger.Warn("cache get failed", slog.Any("error", err), slog.String("key", key))
	}
	if entry.Hit {
		return productFrom(entry)
	}
	return s.loadProduct(ctx, id)
}

// loadProduct coalesces concurrent misses for id into a single DB load.
func (s *Service) loadProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	key := id.String()
	return s.loads.Do(ctx, key, func(ctx context.Context) (domain.Product, error) {
		// re-read inside the flight, because the guard needs a token read from within it
		entry, cacheErr := s.cache.Get(ctx, key)
		if cacheErr != nil {
			s.logger.Warn("cache get failed", slog.Any("error", cacheErr), slog.String("key", key))
		}
		if entry.Hit {
			return productFrom(entry)
		}

		p, err := s.store.FindByID(ctx, id)
		// fill only after a clean Get, because an unread key leaves nothing to fence with
		if cacheErr == nil {
			s.fill(ctx, key, p, entry, err)
		}
		if err != nil {
			return domain.Product{}, err
		}
		return p, nil
	})
}

func (s *Service) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return s.store.FindAll(ctx, cursor, limit)
}

func (s *Service) Update(ctx context.Context, p domain.Product) (domain.Product, error) {
	updated, err := s.store.Update(ctx, p)
	if err != nil {
		return domain.Product{}, err
	}
	s.invalidate(ctx, p.ID)
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(ctx, id)
	return nil
}

// CreateOrder decrements stock and records an order owned by userID in one store tx.
func (s *Service) CreateOrder(
	ctx context.Context, userID domain.UserID, productID uuid.UUID, qty int64,
) (domain.Product, domain.Order, error) {
	orderID, err := uuid.NewV7()
	if err != nil {
		return domain.Product{}, domain.Order{}, fmt.Errorf("failed to generate uuid: %w", err)
	}
	o := domain.Order{ID: domain.OrderID(orderID), UserID: userID, ProductID: productID, Quantity: qty}

	p, placed, err := s.store.CreateOrder(ctx, o)
	if err != nil {
		return domain.Product{}, domain.Order{}, err
	}
	s.invalidate(ctx, productID)
	return p, placed, nil
}

func (s *Service) fill(
	ctx context.Context, key string, p domain.Product, prev cache.Entry, loadErr error,
) {
	var err error
	switch {
	case loadErr == nil:
		err = s.cache.Set(ctx, key, p, prev)
	case errors.Is(loadErr, domain.ErrNotFound):
		err = s.cache.SetMissing(ctx, key, prev)
	default:
		return
	}
	if err != nil {
		s.logger.Warn("cache fill failed", slog.Any("error", err), slog.String("key", key))
	}
}

func (s *Service) invalidate(ctx context.Context, id uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.loadTimeout)
	defer cancel()

	key := id.String()
	if err := s.cache.Invalidate(ctx, key); err != nil {
		s.logger.Warn("cache invalidate failed, entry stale until TTL",
			slog.Any("error", err), slog.String("key", key))
	}
}

// FindOrder returns the caller's own order, foreign ones stay hidden.
func (s *Service) FindOrder(
	ctx context.Context, userID domain.UserID, orderID domain.OrderID,
) (domain.Order, error) {
	return s.store.FindOrder(ctx, userID, orderID)
}

// FindOrders returns a keyset page of the caller's own orders, newest first, uncached.
func (s *Service) FindOrders(
	ctx context.Context, userID domain.UserID, cursor uuid.NullUUID, limit int,
) (domain.OrderPage, error) {
	return s.store.FindOrders(ctx, userID, cursor, limit)
}

func productFrom(e cache.Entry) (domain.Product, error) {
	if !e.Found {
		return domain.Product{}, fmt.Errorf("find product: %w", domain.ErrNotFound)
	}
	return e.Product, nil
}
