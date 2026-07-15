package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/alkmc/storefront/internal/cache"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

func TestService_Create(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		product  domain.Product
		spySetup func(*SpyStore)
		wantErr  bool
	}{
		{
			name:    "success",
			product: domain.Product{Name: "Test", Price: testMoney(1000)},
			spySetup: func(s *SpyStore) {
				s.SaveFn = func(_ context.Context, p domain.Product) (domain.Product, error) {
					return p, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spyStore := new(SpyStore{})
			tt.spySetup(spyStore)
			srv := newTestService(spyStore)

			res, err := srv.Create(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Name != tt.product.Name {
				t.Errorf("got %v, want %v", res.Name, tt.product.Name)
			}
		})
	}
}

func TestService_FindByID(t *testing.T) {
	ctx := t.Context()
	id := uuid.Must(uuid.NewV7())

	tests := []struct {
		name     string
		id       uuid.UUID
		spySetup func(*SpyStore)
		wantErr  bool
	}{
		{
			name: "success",
			id:   id,
			spySetup: func(s *SpyStore) {
				s.FindByIDFn = func(_ context.Context, id uuid.UUID) (domain.Product, error) {
					return domain.Product{ID: id, Name: "Test"}, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spyStore := new(SpyStore{})
			tt.spySetup(spyStore)
			srv := newTestService(spyStore)

			res, err := srv.FindByID(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.ID != tt.id {
				t.Errorf("got %v, want %v", res.ID, tt.id)
			}
		})
	}
}

func TestService_FindByID_CoalescesConcurrentMisses(t *testing.T) {
	tests := []struct {
		name          string
		callers       int
		wantStoreHits int32
	}{
		{
			name:          "all concurrent callers share one store load",
			callers:       100,
			wantStoreHits: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				id := uuid.Must(uuid.NewV7())

				var storeCalls atomic.Int32
				release := make(chan struct{})
				spyStore := &SpyStore{
					FindByIDFn: func(_ context.Context, id uuid.UUID) (domain.Product, error) {
						storeCalls.Add(1)
						<-release
						return domain.Product{ID: id, Price: testMoney(100)}, nil
					},
				}
				srv := newTestService(spyStore)

				var wg sync.WaitGroup
				for range tt.callers {
					wg.Go(func() {
						if _, err := srv.FindByID(t.Context(), id); err != nil {
							t.Errorf("unexpected error: %v", err)
						}
					})
				}
				synctest.Wait()
				close(release)
				wg.Wait()

				if got := storeCalls.Load(); got != tt.wantStoreHits {
					t.Errorf("got %d store calls, want %d", got, tt.wantStoreHits)
				}
			})
		})
	}
}

func TestService_FindAll(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		spySetup func(*SpyStore)
		wantLen  int
		wantErr  bool
	}{
		{
			name: "success",
			spySetup: func(s *SpyStore) {
				s.FindAllFn = func(_ context.Context, _ uuid.NullUUID, _ int) (domain.ProductPage, error) {
					return domain.ProductPage{Items: []domain.Product{
						{Name: "P1", Price: testMoney(100)}, {Name: "P2", Price: testMoney(200)},
					}}, nil
				}
			},
			wantLen: 2,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spyStore := new(SpyStore{})
			tt.spySetup(spyStore)
			srv := newTestService(spyStore)

			page, err := srv.FindAll(ctx, uuid.NullUUID{}, 50)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(page.Items) != tt.wantLen {
				t.Errorf("got length %d, want %d", len(page.Items), tt.wantLen)
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		product  domain.Product
		spySetup func(*SpyStore)
		wantErr  bool
	}{
		{
			name:    "success",
			product: domain.Product{Name: "Update", Price: testMoney(1000)},
			spySetup: func(s *SpyStore) {
				s.UpdateFn = func(_ context.Context, p domain.Product) (domain.Product, error) {
					return p, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spyStore := new(SpyStore{})
			tt.spySetup(spyStore)
			srv := newTestService(spyStore)

			_, err := srv.Update(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestService_Delete(t *testing.T) {
	ctx := t.Context()
	id := uuid.Must(uuid.NewV7())

	tests := []struct {
		name     string
		id       uuid.UUID
		spySetup func(*SpyStore)
		wantErr  bool
	}{
		{
			name: "success",
			id:   id,
			spySetup: func(s *SpyStore) {
				s.DeleteFn = func(_ context.Context, _ uuid.UUID) error {
					return nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spyStore := new(SpyStore{})
			tt.spySetup(spyStore)
			srv := newTestService(spyStore)

			err := srv.Delete(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestService_Purchase(t *testing.T) {
	ctx := t.Context()
	id := uuid.Must(uuid.NewV7())

	spyStore := new(SpyStore{})
	spyStore.PurchaseFn = func(_ context.Context, _ uuid.UUID, _ int64) (domain.Product, error) {
		return domain.Product{ID: id, Stock: 3}, nil
	}
	srv := newTestService(spyStore)

	p, err := srv.Purchase(ctx, id, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Stock != 3 {
		t.Errorf("got remaining stock %d, want 3", p.Stock)
	}
}

func newTestService(s store) *Service {
	return NewService(s, stubCache{}, time.Second, slog.New(slog.DiscardHandler))
}

func testMoney(amount int64) domain.Money {
	return domain.Money{MinorAmount: amount, Currency: domain.CurrencyPLN}
}

func TestService_WritersOnlyInvalidate(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name string
		call func(*Service) error
	}{
		{
			name: "update",
			call: func(s *Service) error {
				_, err := s.Update(t.Context(), domain.Product{ID: id, Price: testMoney(100)})
				return err
			},
		},
		{
			name: "delete",
			call: func(s *Service) error { return s.Delete(t.Context(), id) },
		},
		{
			name: "purchase",
			call: func(s *Service) error {
				_, err := s.Purchase(t.Context(), id, 1)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spyCache := &SpyCache{}
			srv := NewService(&SpyStore{}, spyCache, time.Second, slog.New(slog.DiscardHandler))

			if err := tt.call(srv); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if spyCache.Sets != 0 {
				t.Errorf("wrote %d values to cache, a writer must only invalidate", spyCache.Sets)
			}
			if spyCache.Invalidates != 1 {
				t.Fatalf("Invalidates = %d, want 1", spyCache.Invalidates)
			}
			if got := spyCache.InvalidatedKeys[0]; got != id.String() {
				t.Errorf("invalidated %q, want %q", got, id.String())
			}
		})
	}
}

func TestService_CreateLeavesCacheAlone(t *testing.T) {
	spyCache := &SpyCache{}
	spyStore := &SpyStore{
		SaveFn: func(_ context.Context, p domain.Product) (domain.Product, error) { return p, nil },
	}
	srv := NewService(spyStore, spyCache, time.Second, slog.New(slog.DiscardHandler))

	if _, err := srv.Create(t.Context(), domain.Product{Name: "New", Price: testMoney(100)}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if spyCache.Sets+spyCache.Invalidates != 0 {
		t.Errorf("Create touched the cache: Set=%d Invalidate=%d", spyCache.Sets, spyCache.Invalidates)
	}
}

func TestService_FindByID_CachePaths(t *testing.T) {
	id := uuid.New()
	found := domain.Product{ID: id, Name: "Cached", Price: testMoney(100)}

	tests := []struct {
		name           string
		getFn          func(context.Context, string) (cache.Entry, error)
		findErr        error
		wantStoreCalls int32
		wantSets       int
		wantErr        error
	}{
		{
			name: "known present, store untouched",
			getFn: func(context.Context, string) (cache.Entry, error) {
				return cache.Entry{Product: found, Hit: true}, nil
			},
		},
		{
			name:           "miss populates",
			getFn:          func(context.Context, string) (cache.Entry, error) { return cache.Entry{}, nil },
			wantStoreCalls: 1,
			wantSets:       1,
		},
		{
			name: "cache unreadable, nothing populated",
			getFn: func(context.Context, string) (cache.Entry, error) {
				return cache.Entry{}, errors.New("redis down")
			},
			wantStoreCalls: 1,
		},
		{
			name:           "store fails, nothing populated",
			getFn:          func(context.Context, string) (cache.Entry, error) { return cache.Entry{}, nil },
			findErr:        domain.ErrUnavailable,
			wantStoreCalls: 1,
			wantErr:        domain.ErrUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var storeCalls atomic.Int32
			spyCache := &SpyCache{GetFn: tt.getFn}
			spyStore := &SpyStore{
				FindByIDFn: func(_ context.Context, id uuid.UUID) (domain.Product, error) {
					storeCalls.Add(1)
					if tt.findErr != nil {
						return domain.Product{}, tt.findErr
					}
					return domain.Product{ID: id, Price: testMoney(100)}, nil
				},
			}
			srv := NewService(spyStore, spyCache, time.Second, slog.New(slog.DiscardHandler))

			_, err := srv.FindByID(t.Context(), id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("FindByID error = %v, want %v", err, tt.wantErr)
			}
			if got := storeCalls.Load(); got != tt.wantStoreCalls {
				t.Errorf("store calls = %d, want %d", got, tt.wantStoreCalls)
			}
			if spyCache.Sets != tt.wantSets {
				t.Errorf("Sets = %d, want %d", spyCache.Sets, tt.wantSets)
			}
		})
	}
}

func TestService_InvalidateSurvivesCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	spyCache := &SpyCache{}
	srv := NewService(&SpyStore{}, spyCache, time.Second, slog.New(slog.DiscardHandler))

	if err := srv.Delete(ctx, uuid.New()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if spyCache.Invalidates != 1 {
		t.Fatalf("Invalidates = %d, want 1 even though the caller went away", spyCache.Invalidates)
	}
	if spyCache.InvalidateCtxErr != nil {
		t.Errorf("invalidation ran on a dead context (%v), it must be detached",
			spyCache.InvalidateCtxErr)
	}
}
