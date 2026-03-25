// Package types defines the shared request/response structs used by both
// the REST and gRPC benchmark clients. Must match the server's types.go.
package types

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
