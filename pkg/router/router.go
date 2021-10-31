package router

import "net/http"

// Router is responsible for matching uri with application controller
type Router interface {
	POST(string, func(w http.ResponseWriter, r *http.Request))
	GET(string, func(w http.ResponseWriter, r *http.Request))
	PUT(string, func(w http.ResponseWriter, r *http.Request))
	DELETE(string, func(w http.ResponseWriter, r *http.Request))
	SERVE(string)
}
