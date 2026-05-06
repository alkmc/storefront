package service

import (
	"context"
	"testing"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
)

type MockRepository struct {
	SaveFn     func(ctx context.Context, p *entity.Product) (*entity.Product, error)
	FindByIDFn func(ctx context.Context, id uuid.UUID) (*entity.Product, error)
	FindAllFn  func(ctx context.Context) ([]entity.Product, error)
	UpdateFn   func(ctx context.Context, p *entity.Product) error
	DeleteFn   func(ctx context.Context, id uuid.UUID) error
	CloseDBFn  func()
}

func (m *MockRepository) Save(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	if m.SaveFn != nil {
		return m.SaveFn(ctx, p)
	}
	return nil, nil
}

func (m *MockRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	if m.FindByIDFn != nil {
		return m.FindByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *MockRepository) FindAll(ctx context.Context) ([]entity.Product, error) {
	if m.FindAllFn != nil {
		return m.FindAllFn(ctx)
	}
	return nil, nil
}

func (m *MockRepository) Update(ctx context.Context, p *entity.Product) error {
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

func (m *MockRepository) CloseDB() {
	if m.CloseDBFn != nil {
		m.CloseDBFn()
	}
}

func TestService_Create(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		product   *entity.Product
		mockSetup func(*MockRepository)
		wantErr   bool
	}{
		{
			name:    "success",
			product: new(entity.Product{Name: "Test", Price: 10.0}),
			mockSetup: func(m *MockRepository) {
				m.SaveFn = func(ctx context.Context, p *entity.Product) (*entity.Product, error) {
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
			srv := NewService(mockRepo)

			res, err := srv.Create(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.Name != tt.product.Name {
					t.Errorf("got %v, want %v", res.Name, tt.product.Name)
				}
			}
		})
	}
}

func TestService_FindByID(t *testing.T) {
	ctx := t.Context()
	id := uuid.New()

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
				m.FindByIDFn = func(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
					return new(entity.Product{ID: id, Name: "Test"}), nil
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockRepository{})
			tt.mockSetup(mockRepo)
			srv := NewService(mockRepo)

			res, err := srv.FindByID(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.ID != tt.id {
					t.Errorf("got %v, want %v", res.ID, tt.id)
				}
			}
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
				m.FindAllFn = func(ctx context.Context) ([]entity.Product, error) {
					return []entity.Product{{Name: "P1"}, {Name: "P2"}}, nil
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
			srv := NewService(mockRepo)

			res, err := srv.FindAll(ctx)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(res) != tt.wantLen {
					t.Errorf("got length %d, want %d", len(res), tt.wantLen)
				}
			}
		})
	}
}

func TestService_Update(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name      string
		product   *entity.Product
		mockSetup func(*MockRepository)
		wantErr   bool
	}{
		{
			name:    "success",
			product: new(entity.Product{Name: "Update"}),
			mockSetup: func(m *MockRepository) {
				m.UpdateFn = func(ctx context.Context, p *entity.Product) error {
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
			srv := NewService(mockRepo)

			err := srv.Update(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestService_Delete(t *testing.T) {
	ctx := t.Context()
	id := uuid.New()

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
				m.DeleteFn = func(ctx context.Context, id uuid.UUID) error {
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
			srv := NewService(mockRepo)

			err := srv.Delete(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
