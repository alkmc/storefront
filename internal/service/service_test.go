package service

import (
	"context"
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

type mockCache struct{}

func (mockCache) Set(_ context.Context, _ string, _ domain.Product) error {
	return nil
}

func (mockCache) Get(_ context.Context, _ string) (domain.Product, error) {
	return domain.Product{}, cache.ErrCacheMiss
}

func (mockCache) Invalidate(_ context.Context, _ string) error {
	return nil
}

func newTestService(s store) *Service {
	return NewService(s, mockCache{}, time.Second, slog.New(slog.DiscardHandler))
}

func testMoney(amount int64) domain.Money {
	return domain.Money{MinorAmount: amount, Currency: domain.CurrencyPLN}
}

type MockStore struct {
	SaveFn     func(context.Context, domain.Product) (domain.Product, error)
	FindByIDFn func(context.Context, uuid.UUID) (domain.Product, error)
	FindAllFn  func(context.Context, uuid.NullUUID, int) (domain.ProductPage, error)
	UpdateFn   func(context.Context, domain.Product) error
	DeleteFn   func(context.Context, uuid.UUID) error
}

func (m *MockStore) Save(ctx context.Context, p domain.Product) (domain.Product, error) {
	return m.SaveFn(ctx, p)
}

func (m *MockStore) FindByID(ctx context.Context, id uuid.UUID) (domain.Product, error) {
	return m.FindByIDFn(ctx, id)
}

func (m *MockStore) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	return m.FindAllFn(ctx, cursor, limit)
}

func (m *MockStore) Update(ctx context.Context, p domain.Product) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, p)
	}
	return nil
}

func (m *MockStore) Delete(ctx context.Context, id uuid.UUID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return nil
}

func TestService_Create(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		product   domain.Product
		mockSetup func(*MockStore)
		wantErr   bool
	}{
		{
			name:    "success",
			product: domain.Product{Name: "Test", Price: testMoney(1000)},
			mockSetup: func(m *MockStore) {
				m.SaveFn = func(_ context.Context, p domain.Product) (domain.Product, error) {
					return p, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStore{})
			tt.mockSetup(mockStore)
			srv := newTestService(mockStore)

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
		name      string
		id        uuid.UUID
		mockSetup func(*MockStore)
		wantErr   bool
	}{
		{
			name: "success",
			id:   id,
			mockSetup: func(m *MockStore) {
				m.FindByIDFn = func(_ context.Context, id uuid.UUID) (domain.Product, error) {
					return domain.Product{ID: id, Name: "Test"}, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStore{})
			tt.mockSetup(mockStore)
			srv := newTestService(mockStore)

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
				mockStore := &MockStore{
					FindByIDFn: func(_ context.Context, id uuid.UUID) (domain.Product, error) {
						storeCalls.Add(1)
						<-release
						return domain.Product{ID: id, Price: testMoney(100)}, nil
					},
				}
				srv := newTestService(mockStore)

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
		name      string
		mockSetup func(*MockStore)
		wantLen   int
		wantErr   bool
	}{
		{
			name: "success",
			mockSetup: func(m *MockStore) {
				m.FindAllFn = func(_ context.Context, _ uuid.NullUUID, _ int) (domain.ProductPage, error) {
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
			mockStore := new(MockStore{})
			tt.mockSetup(mockStore)
			srv := newTestService(mockStore)

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
		name      string
		product   domain.Product
		mockSetup func(*MockStore)
		wantErr   bool
	}{
		{
			name:    "success",
			product: domain.Product{Name: "Update", Price: testMoney(1000)},
			mockSetup: func(m *MockStore) {
				m.UpdateFn = func(_ context.Context, _ domain.Product) error {
					return nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStore{})
			tt.mockSetup(mockStore)
			srv := newTestService(mockStore)

			err := srv.Update(ctx, tt.product)
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
		name      string
		id        uuid.UUID
		mockSetup func(*MockStore)
		wantErr   bool
	}{
		{
			name: "success",
			id:   id,
			mockSetup: func(m *MockStore) {
				m.DeleteFn = func(_ context.Context, _ uuid.UUID) error {
					return nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := new(MockStore{})
			tt.mockSetup(mockStore)
			srv := newTestService(mockStore)

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
