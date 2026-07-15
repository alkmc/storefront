package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

type (
	store interface {
		Save(context.Context, domain.Product) (domain.Product, error)
		FindByID(context.Context, uuid.UUID) (domain.Product, error)
		FindAll(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
		Update(context.Context, domain.Product) (domain.Product, error)
		Delete(context.Context, uuid.UUID) error
		Purchase(context.Context, uuid.UUID, int64) (domain.Product, error)
	}
	cacher interface {
		Set(context.Context, string, domain.Product, cache.Entry) error
		Get(context.Context, string) (cache.Entry, error)
		Invalidate(context.Context, string) error
	}
	Service struct {
		logger      *slog.Logger
		store       store
		cache       cacher
		loadGroup   singleflight.Group
		loadTimeout time.Duration
	}
)

// NewService initializes the service with the given store and cache.
// loadTimeout caps a single detached store read and cache.Set.
func NewService(s store, c cacher, loadTimeout time.Duration, l *slog.Logger) *Service {
	return new(Service{logger: l, store: s, cache: c, loadTimeout: loadTimeout})
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
		return entry.Product, nil
	}
	return s.loadProduct(ctx, id)
}

// loadProduct coalesces concurrent misses for id into a single DB load via singleflight.
func (s *Service) loadProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	key := id.String()
	v, err, _ := s.loadGroup.Do(key, func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.loadTimeout)
		defer cancel()

		// re-read inside the flight, because the guard needs a token read from within it.
		entry, cacheErr := s.cache.Get(loadCtx, key)
		if cacheErr != nil {
			s.logger.Warn("cache get failed", slog.Any("error", cacheErr), slog.String("key", key))
		}
		if entry.Hit {
			return entry.Product, nil
		}

		p, err := s.store.FindByID(loadCtx, id)
		if err != nil {
			return domain.Product{}, err
		}
		// populate only after a clean Get, because an unread key leaves nothing to fence with.
		if cacheErr == nil {
			if err := s.cache.Set(loadCtx, key, p, entry); err != nil {
				s.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
			}
		}
		return p, nil
	})
	if err != nil {
		return domain.Product{}, err
	}
	p, ok := v.(domain.Product)
	if !ok {
		return domain.Product{}, fmt.Errorf("singleflight: unexpected result type %T", v)
	}
	return p, nil
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

func (s *Service) Purchase(ctx context.Context, id uuid.UUID, qty int64) (domain.Product, error) {
	p, err := s.store.Purchase(ctx, id, qty)
	if err != nil {
		return domain.Product{}, err
	}
	s.invalidate(ctx, id)
	return p, nil
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
