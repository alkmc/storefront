package serviceerr

import (
	"fmt"
)

const (
	valErr    = "validation error"
	codecErr  = "JSON error"
	internErr = "service error"
	userErr   = "invalid input error"
)

// ServiceError shall be used to return business error messages
type ServiceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s *ServiceError) Error() string {
	return fmt.Sprintf("%s -  %s", s.Code, s.Message)
}

//Internal constructs internal service error
func Internal(msg string) *ServiceError {
	return &ServiceError{
		Code:    internErr,
		Message: msg,
	}
}

//Codec constructs JSON error
func Codec(msg string) *ServiceError {
	return &ServiceError{
		Code:    codecErr,
		Message: msg,
	}
}

//Valid constructs validation error
func Valid(msg string) *ServiceError {
	return &ServiceError{
		Code:    valErr,
		Message: msg,
	}
}

//Input constructs invalid user input error
func Input(msg string) *ServiceError {
	return &ServiceError{
		Code:    userErr,
		Message: msg,
	}
}
