package service

import (
	"testing"

	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Save(p *entity.Product) (*entity.Product, error) {
	args := m.Called()
	result := args.Get(0)
	return result.(*entity.Product), args.Error(1)
}

func (m *MockRepository) FindByID(id uuid.UUID) (*entity.Product, error) {
	args := m.Called()
	result := args.Get(0)
	return result.(*entity.Product), args.Error(1)
}

func (m *MockRepository) FindAll() ([]entity.Product, error) {
	args := m.Called()
	result := args.Get(0)
	return result.([]entity.Product), args.Error(1)
}

func (m *MockRepository) Update(p *entity.Product) error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRepository) Delete(id uuid.UUID) error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockRepository) CloseDB() {
}

func TestCreate(t *testing.T) {
	mockRepo := new(MockRepository)
	p := entity.Product{Name: "Created", Price: 1.1}

	mockRepo.On("Save").Return(&p, nil)
	testService := NewService(mockRepo)
	result, err := testService.Create(&p)

	mockRepo.AssertExpectations(t)

	assert.NotNil(t, result.ID)
	assert.Equal(t, "Created", result.Name)
	assert.Equal(t, 1.1, result.Price)
	assert.Nil(t, err)
}

func TestFindByID(t *testing.T) {
	mockRepo := new(MockRepository)
	id := uuid.New()
	p := entity.Product{ID: id, Name: "One", Price: 1.1}

	mockRepo.On("FindByID").Return(&p, nil)
	testService := NewService(mockRepo)
	result, err := testService.FindByID(id)

	mockRepo.AssertExpectations(t)

	assert.Equal(t, id, result.ID)
	assert.Equal(t, "One", result.Name)
	assert.Equal(t, 1.1, result.Price)
	assert.Nil(t, err)
}

func TestFinalAll(t *testing.T) {
	mockRepo := new(MockRepository)
	id := uuid.New()
	p := entity.Product{ID: id, Name: "All", Price: 2.2}

	mockRepo.On("FindAll").Return([]entity.Product{p}, nil)
	testService := NewService(mockRepo)
	result, err := testService.FindAll()

	mockRepo.AssertExpectations(t)

	assert.Equal(t, id, result[0].ID)
	assert.Equal(t, "All", result[0].Name)
	assert.Equal(t, 2.2, result[0].Price)
	assert.Nil(t, err)
}

func TestUpdate(t *testing.T) {
	mockRepo := new(MockRepository)
	id := uuid.New()
	p := entity.Product{ID: id, Name: "Created", Price: 1.1}

	mockRepo.On("Update").Return(nil)

	testService := NewService(mockRepo)
	err := testService.Update(&p)

	mockRepo.AssertExpectations(t)

	assert.Nil(t, err)
}

func TestDelete(t *testing.T) {
	mockRepo := new(MockRepository)
	id := uuid.New()

	mockRepo.On("Delete").Return(nil)
	testService := NewService(mockRepo)
	err := testService.Delete(id)

	mockRepo.AssertExpectations(t)

	assert.Nil(t, err)
}
