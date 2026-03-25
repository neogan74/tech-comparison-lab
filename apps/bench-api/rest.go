package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func newRESTMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /echo", echoHandler)
	mux.HandleFunc("GET /users/{id}", getUserHandler)
	mux.HandleFunc("POST /orders", createOrderHandler)
	return mux
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	msg := r.URL.Query().Get("msg")
	writeJSON(w, handleEcho(EchoReq{Msg: msg}))
}

func getUserHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	writeJSON(w, handleGetUser(UserReq{ID: id}))
}

func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	var req OrderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, handleCreateOrder(req))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
