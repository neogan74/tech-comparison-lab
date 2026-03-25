package main

// Shared request/response types used by both REST and gRPC handlers.
// JSON tags are used for REST; gRPC uses the custom JSON codec.

type EchoReq struct {
	Msg string `json:"msg"`
}
type EchoResp struct {
	Msg string `json:"msg"`
}

type UserReq struct {
	ID int `json:"id"`
}
type UserResp struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Country string `json:"country"`
	Score   int    `json:"score"`
}

type OrderReq struct {
	UserID    int `json:"user_id"`
	ProductID int `json:"product_id"`
	Qty       int `json:"qty"`
}
type OrderResp struct {
	ID     int     `json:"id"`
	Status string  `json:"status"`
	Total  float64 `json:"total"`
}

// --- Deterministic in-memory handlers (same logic for REST and gRPC) ---

var countries = []string{"US", "GB", "DE", "FR", "JP", "CA", "AU", "IN", "BR", "MX"}
var names = []string{"Alice", "Bob", "Carol", "Dave", "Eve", "Frank", "Grace", "Hank"}

func handleEcho(req EchoReq) EchoResp {
	return EchoResp{Msg: req.Msg}
}

func handleGetUser(req UserReq) UserResp {
	id := req.ID
	if id <= 0 {
		id = 1
	}
	return UserResp{
		ID:      id,
		Name:    names[id%len(names)],
		Email:   names[id%len(names)] + "@bench.local",
		Country: countries[id%len(countries)],
		Score:   (id * 31) % 1000,
	}
}

func handleCreateOrder(req OrderReq) OrderResp {
	id := req.UserID*1000 + req.ProductID
	return OrderResp{
		ID:     id,
		Status: "confirmed",
		Total:  float64(req.Qty) * 9.99,
	}
}
