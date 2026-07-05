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
	"golang.org/x/sync/singleflight"
)

type (
	store interface {
		Save(context.Context, domain.Product) (domain.Product, error)
		FindByID(context.Context, uuid.UUID) (domain.Product, error)
		FindAll(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
		Update(context.Context, domain.Product) error
		Delete(context.Context, uuid.UUID) error
	}
	cacher interface {
		Set(context.Context, string, domain.Product) error
		Get(context.Context, string) (domain.Product, error)
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
func NewService(l *slog.Logger, s store, c cacher, loadTimeout time.Duration) *Service {
	return new(Service{logger: l, store: s, cache: c, loadTimeout: loadTimeout})
}

func (s *Service) Create(ctx context.Context, p domain.Product) (domain.Product, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return domain.Product{}, fmt.Errorf("failed to generate uuid: %w", err)
	}
	p.ID = id
	saved, err := s.store.Save(ctx, p)
	if err != nil {
		return domain.Product{}, err
	}
	key := saved.ID.String()
	if err := s.cache.Set(ctx, key, saved); err != nil {
		s.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
	}
	return saved, nil
}

func (s *Service) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	key := id.String()
	cached, err := s.cache.Get(ctx, key)
	if err == nil {
		return cached, nil
	}
	if !errors.Is(err, cache.ErrCacheMiss) {
		s.logger.Warn("cache get failed", slog.Any("error", err), slog.String("key", key))
	}
	return s.loadProduct(ctx, id)
}

// loadProduct coalesces concurrent misses for id into a single DB load via singleflight.
func (s *Service) loadProduct(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	key := id.String()
	v, err, _ := s.loadGroup.Do(key, func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.loadTimeout)
		defer cancel()

		p, err := s.store.FindByID(loadCtx, id)
		if err != nil {
			return domain.Product{}, err
		}
		if err := s.cache.Set(loadCtx, key, p); err != nil {
			s.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
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

func (s *Service) Update(ctx context.Context, p domain.Product) error {
	if err := s.store.Update(ctx, p); err != nil {
		return err
	}
	key := p.ID.String()
	if err := s.cache.Invalidate(ctx, key); err != nil {
		s.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", key))
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	key := id.String()
	if err := s.cache.Invalidate(ctx, key); err != nil {
		s.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", key))
	}
	return nil
}
