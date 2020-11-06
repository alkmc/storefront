package serviceerr

import (
	"fmt"
	"net/http"

	"github.com/alkmc/restClean/renderer"
)

const (
	valErr     = "validation error"
	codecErr   = "JSON error"
	internErr  = "service error"
	userErr    = "invalid input error"
	payloadErr = "Request body error"
)

// ServiceError shall be used to return business error messages
type ServiceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (s *ServiceError) Error() string {
	return fmt.Sprintf("%s -  %s", s.Code, s.Message)
}

//Encode is similar to http.Error, but response is encoded in JSON format
func (s *ServiceError) Encode(w http.ResponseWriter) {
	codes := map[string]int{
		valErr:     http.StatusBadRequest,
		userErr:    http.StatusBadRequest,
		payloadErr: http.StatusUnprocessableEntity,
	}
	code, ok := codes[s.Code]
	if !ok {
		code = http.StatusInternalServerError
	}
	renderer.JSON(w, code, s)
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

//Body constructs request payload error
func Body(msg string) *ServiceError {
	return &ServiceError{
		Code:    payloadErr,
		Message: msg,
	}
}
