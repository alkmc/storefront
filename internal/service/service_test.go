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
	"github.com/alkmc/storefront/internal/entity"
	"github.com/google/uuid"
)

type mockCache struct{}

func (mockCache) Set(_ context.Context, _ string, _ entity.Product) error {
	return nil
}

func (mockCache) Get(_ context.Context, _ string) (entity.Product, error) {
	return entity.Product{}, cache.ErrCacheMiss
}

func (mockCache) Invalidate(_ context.Context, _ string) error {
	return nil
}

func newTestService(repo repository) *Service {
	return NewService(slog.New(slog.DiscardHandler), repo, mockCache{}, time.Second)
}

func testMoney(amount int64) entity.Money {
	return entity.Money{MinorAmount: amount, Currency: entity.CurrencyPLN}
}

type MockRepository struct {
	SaveFn     func(ctx context.Context, p entity.Product) (entity.Product, error)
	FindByIDFn func(ctx context.Context, id uuid.UUID) (entity.Product, error)
	FindAllFn  func(ctx context.Context, limit, offset int) ([]entity.Product, error)
	UpdateFn   func(ctx context.Context, p entity.Product) error
	DeleteFn   func(ctx context.Context, id uuid.UUID) error
}

func (m *MockRepository) Save(ctx context.Context, p entity.Product) (entity.Product, error) {
	return m.SaveFn(ctx, p)
}

func (m *MockRepository) FindByID(ctx context.Context, id uuid.UUID) (entity.Product, error) {
	return m.FindByIDFn(ctx, id)
}

func (m *MockRepository) FindAll(ctx context.Context, limit, offset int) ([]entity.Product, error) {
	return m.FindAllFn(ctx, limit, offset)
}

func (m *MockRepository) Update(ctx context.Context, p entity.Product) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, p)
	}
	return nil
}

func (m *MockRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id)
	}
	return nil
}

func TestService_Create(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		product   entity.Product
		mockSetup func(*MockRepository)
		wantErr   bool
	}{
		{
			name:    "success",
			product: entity.Product{Name: "Test", Price: testMoney(1000)},
			mockSetup: func(m *MockRepository) {
				m.SaveFn = func(_ context.Context, p entity.Product) (entity.Product, error) {
					return p, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository{})
			tt.mockSetup(mockRepo)
			srv := newTestService(mockRepo)

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
		mockSetup func(*MockRepository)
		wantErr   bool
	}{
		{
			name: "success",
			id:   id,
			mockSetup: func(m *MockRepository) {
				m.FindByIDFn = func(_ context.Context, id uuid.UUID) (entity.Product, error) {
					return entity.Product{ID: id, Name: "Test"}, nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository{})
			tt.mockSetup(mockRepo)
			srv := newTestService(mockRepo)

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
		name         string
		callers      int
		wantRepoHits int32
	}{
		{
			name:         "all concurrent callers share one repo load",
			callers:      100,
			wantRepoHits: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				id := uuid.Must(uuid.NewV7())

				var repoCalls atomic.Int32
				release := make(chan struct{})
				mockRepo := &MockRepository{
					FindByIDFn: func(_ context.Context, id uuid.UUID) (entity.Product, error) {
						repoCalls.Add(1)
						<-release
						return entity.Product{ID: id, Price: testMoney(100)}, nil
					},
				}
				srv := newTestService(mockRepo)

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

				if got := repoCalls.Load(); got != tt.wantRepoHits {
					t.Errorf("got %d repo calls, want %d", got, tt.wantRepoHits)
				}
			})
		})
	}
}

func TestService_FindAll(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		mockSetup func(*MockRepository)
		wantLen   int
		wantErr   bool
	}{
		{
			name: "success",
			mockSetup: func(m *MockRepository) {
				m.FindAllFn = func(_ context.Context, _, _ int) ([]entity.Product, error) {
					return []entity.Product{{Name: "P1", Price: testMoney(100)}, {Name: "P2", Price: testMoney(200)}}, nil
				}
			},
			wantLen: 2,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository{})
			tt.mockSetup(mockRepo)
			srv := newTestService(mockRepo)

			res, err := srv.FindAll(ctx, 50, 0)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res) != tt.wantLen {
				t.Errorf("got length %d, want %d", len(res), tt.wantLen)
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		product   entity.Product
		mockSetup func(*MockRepository)
		wantErr   bool
	}{
		{
			name:    "success",
			product: entity.Product{Name: "Update", Price: testMoney(1000)},
			mockSetup: func(m *MockRepository) {
				m.UpdateFn = func(_ context.Context, _ entity.Product) error {
					return nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository{})
			tt.mockSetup(mockRepo)
			srv := newTestService(mockRepo)

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
		mockSetup func(*MockRepository)
		wantErr   bool
	}{
		{
			name: "success",
			id:   id,
			mockSetup: func(m *MockRepository) {
				m.DeleteFn = func(_ context.Context, _ uuid.UUID) error {
					return nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository{})
			tt.mockSetup(mockRepo)
			srv := newTestService(mockRepo)

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
