package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/entity"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

type (
	repository interface {
		Save(context.Context, entity.Product) (entity.Product, error)
		FindByID(context.Context, uuid.UUID) (entity.Product, error)
		FindAll(context.Context, uuid.NullUUID, int) (entity.ProductPage, error)
		Update(context.Context, entity.Product) error
		Delete(context.Context, uuid.UUID) error
	}
	cacher interface {
		Set(context.Context, string, entity.Product) error
		Get(context.Context, string) (entity.Product, error)
		Invalidate(context.Context, string) error
	}
	Service struct {
		logger      *slog.Logger
		repo        repository
		cache       cacher
		loadGroup   singleflight.Group
		loadTimeout time.Duration
	}
)

// NewService initializes the business logic layer backed by the provided repository and cache.
// loadTimeout caps a single repo+cache.Set roundtrip after the caller's context is detached
// via context.WithoutCancel inside loadProduct.
func NewService(l *slog.Logger, r repository, c cacher, loadTimeout time.Duration) *Service {
	return new(Service{logger: l, repo: r, cache: c, loadTimeout: loadTimeout})
}

func (s *Service) Create(ctx context.Context, p entity.Product) (entity.Product, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return entity.Product{}, fmt.Errorf("failed to generate uuid: %w", err)
	}
	p.ID = id
	saved, err := s.repo.Save(ctx, p)
	if err != nil {
		return entity.Product{}, err
	}
	key := saved.ID.String()
	if err := s.cache.Set(ctx, key, saved); err != nil {
		s.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
	}
	return saved, nil
}

func (s *Service) FindByID(ctx context.Context, id uuid.UUID) (entity.Product, error) {
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
func (s *Service) loadProduct(ctx context.Context, id uuid.UUID) (entity.Product, error) {
	key := id.String()
	v, err, _ := s.loadGroup.Do(key, func() (any, error) {
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.loadTimeout)
		defer cancel()

		p, err := s.repo.FindByID(loadCtx, id)
		if err != nil {
			return entity.Product{}, err
		}
		if err := s.cache.Set(loadCtx, key, p); err != nil {
			s.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
		}
		return p, nil
	})
	if err != nil {
		return entity.Product{}, err
	}
	p, ok := v.(entity.Product)
	if !ok {
		return entity.Product{}, fmt.Errorf("singleflight: unexpected result type %T", v)
	}
	return p, nil
}

func (s *Service) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (entity.ProductPage, error) {
	return s.repo.FindAll(ctx, cursor, limit)
}

func (s *Service) Update(ctx context.Context, p entity.Product) error {
	if err := s.repo.Update(ctx, p); err != nil {
		return err
	}
	key := p.ID.String()
	if err := s.cache.Invalidate(ctx, key); err != nil {
		s.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", key))
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	key := id.String()
	if err := s.cache.Invalidate(ctx, key); err != nil {
		s.logger.Warn("cache invalidate failed", slog.Any("error", err), slog.String("key", key))
	}
	return nil
}
