package controller

import "net/http"

// Controller is responsible for controlling the application logic
type Controller interface {
	GetByID(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Add(w http.ResponseWriter, r *http.Request)
	Delete(w http.ResponseWriter, r *http.Request)
	Update(w http.ResponseWriter, r *http.Request)
}
