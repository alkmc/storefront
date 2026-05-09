package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alkmc/restClean/internal/cache"
	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type (
	repository interface {
		Save(context.Context, entity.Product) (entity.Product, error)
		FindByID(context.Context, uuid.UUID) (entity.Product, error)
		FindAll(ctx context.Context, limit, offset int) ([]entity.Product, error)
		Update(context.Context, entity.Product) error
		Delete(context.Context, uuid.UUID) error
	}

	cacher interface {
		Set(ctx context.Context, key string, value entity.Product) error
		Get(ctx context.Context, key string) (entity.Product, error)
		Invalidate(ctx context.Context, key string) error
	}

	Service struct {
		logger *slog.Logger
		repo   repository
		cache  cacher
	}
)

// NewService initializes the business logic layer backed by the provided repository and cache.
func NewService(l *slog.Logger, r repository, c cacher) *Service {
	return new(Service{logger: l, repo: r, cache: c})
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
	if err := s.cache.Set(ctx, saved.ID.String(), saved); err != nil {
		s.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", saved.ID.String()))
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

	p, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return entity.Product{}, err
	}
	if err := s.cache.Set(ctx, key, p); err != nil {
		s.logger.Warn("cache set failed", slog.Any("error", err), slog.String("key", key))
	}
	return p, nil
}

func (s *Service) FindAll(ctx context.Context, limit, offset int) ([]entity.Product, error) {
	return s.repo.FindAll(ctx, limit, offset)
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
