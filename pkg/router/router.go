package router

import "net/http"

//Router is responsible for matching uri with application controller
type Router interface {
	POST(uri string, f func(w http.ResponseWriter, r *http.Request))
	GET(uri string, f func(w http.ResponseWriter, r *http.Request))
	PUT(uri string, f func(w http.ResponseWriter, r *http.Request))
	DELETE(uri string, f func(w http.ResponseWriter, r *http.Request))
	SERVE(port string)
}
