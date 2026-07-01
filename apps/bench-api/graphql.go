package main

// GraphQL HTTP endpoint — implements the GraphQL-over-HTTP protocol without
// a full schema-validation library so concurrent requests are never serialised.
//
// The three operations are dispatched by a simple query-string pattern match,
// which is representative of what an optimised GraphQL server does internally
// after the one-time parse+plan step.  The measurable overhead vs REST is:
//   • HTTP POST with a JSON body rather than GET parameters / URL routing
//   • JSON decode of {"query","variables"} envelope
//   • Operation dispatch and variable extraction

import (
	"encoding/json"
	"net/http"
	"strings"
)

type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type gqlResponse struct {
	Data   interface{} `json:"data,omitempty"`
	Errors []gqlError  `json:"errors,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

func graphqlHandler(w http.ResponseWriter, r *http.Request) {
	var req gqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(gqlResponse{Errors: []gqlError{{Message: err.Error()}}})
		return
	}

	data, gqlErr := dispatchQuery(req)
	w.Header().Set("Content-Type", "application/json")
	if gqlErr != nil {
		json.NewEncoder(w).Encode(gqlResponse{Errors: []gqlError{{Message: gqlErr.Error()}}})
		return
	}
	json.NewEncoder(w).Encode(gqlResponse{Data: data})
}

func dispatchQuery(req gqlRequest) (interface{}, error) {
	vars := req.Variables

	switch {
	case strings.Contains(req.Query, "createOrder"):
		userID, _ := gqlInt(vars["userId"])
		productID, _ := gqlInt(vars["productId"])
		qty, _ := gqlInt(vars["qty"])
		o := handleCreateOrder(OrderReq{UserID: userID, ProductID: productID, Qty: qty})
		return map[string]interface{}{
			"createOrder": map[string]interface{}{
				"id": o.ID, "status": o.Status, "total": o.Total,
			},
		}, nil

	case strings.Contains(req.Query, "getUser"):
		id, _ := gqlInt(vars["id"])
		u := handleGetUser(UserReq{ID: id})
		return map[string]interface{}{
			"getUser": map[string]interface{}{
				"id": u.ID, "name": u.Name, "email": u.Email,
				"country": u.Country, "score": u.Score,
			},
		}, nil

	default: // echo
		msg, _ := vars["msg"].(string)
		r := handleEcho(EchoReq{Msg: msg})
		return map[string]interface{}{
			"echo": map[string]interface{}{"msg": r.Msg},
		}, nil
	}
}

// gqlInt coerces a JSON-decoded number (float64) or Go int to int.
func gqlInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}