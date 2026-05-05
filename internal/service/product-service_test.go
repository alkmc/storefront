package service

import (
	"context"
	"testing"

	"github.com/alkmc/restClean/internal/entity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Save(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	args := m.Called(ctx, p)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Product), args.Error(1)
}

func (m *MockRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Product), args.Error(1)
}

func (m *MockRepository) FindAll(ctx context.Context) ([]entity.Product, error) {
	args := m.Called(ctx)
	return args.Get(0).([]entity.Product), args.Error(1)
}

func (m *MockRepository) Update(ctx context.Context, p *entity.Product) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

func (m *MockRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRepository) CloseDB() {}

func TestProductService(t *testing.T) {
	mockRepo := new(MockRepository)
	srv := NewService(mockRepo)
	ctx := t.Context()

	t.Run("Create", func(t *testing.T) {
		p := &entity.Product{Name: "Test", Price: 10.0}
		mockRepo.On("Save", ctx, mock.AnythingOfType("*entity.Product")).Return(p, nil).Once()

		res, err := srv.Create(ctx, p)
		assert.NoError(t, err)
		assert.Equal(t, "Test", res.Name)
		mockRepo.AssertExpectations(t)
	})

	t.Run("FindByID", func(t *testing.T) {
		id := uuid.New()
		p := &entity.Product{ID: id, Name: "Test"}
		mockRepo.On("FindByID", ctx, id).Return(p, nil).Once()

		res, err := srv.FindByID(ctx, id)
		assert.NoError(t, err)
		assert.Equal(t, id, res.ID)
		mockRepo.AssertExpectations(t)
	})

	t.Run("FindAll", func(t *testing.T) {
		products := []entity.Product{{Name: "P1"}, {Name: "P2"}}
		mockRepo.On("FindAll", ctx).Return(products, nil).Once()

		res, err := srv.FindAll(ctx)
		assert.NoError(t, err)
		assert.Len(t, res, 2)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Update", func(t *testing.T) {
		p := &entity.Product{Name: "Update"}
		mockRepo.On("Update", ctx, p).Return(nil).Once()

		err := srv.Update(ctx, p)
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Delete", func(t *testing.T) {
		id := uuid.New()
		mockRepo.On("Delete", ctx, id).Return(nil).Once()

		err := srv.Delete(ctx, id)
		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})
}
