package main

import (
	"encoding/json"
	"net/http"

	"github.com/graphql-go/graphql"
)

var gqlSchema graphql.Schema

func init() {
	echoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "EchoResponse",
		Fields: graphql.Fields{
			"msg": &graphql.Field{Type: graphql.String},
		},
	})

	userType := graphql.NewObject(graphql.ObjectConfig{
		Name: "User",
		Fields: graphql.Fields{
			"id":      &graphql.Field{Type: graphql.Int},
			"name":    &graphql.Field{Type: graphql.String},
			"email":   &graphql.Field{Type: graphql.String},
			"country": &graphql.Field{Type: graphql.String},
			"score":   &graphql.Field{Type: graphql.Int},
		},
	})

	orderType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Order",
		Fields: graphql.Fields{
			"id":     &graphql.Field{Type: graphql.Int},
			"status": &graphql.Field{Type: graphql.String},
			"total":  &graphql.Field{Type: graphql.Float},
		},
	})

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"echo": &graphql.Field{
				Type: echoType,
				Args: graphql.FieldConfigArgument{
					"msg": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					msg, _ := p.Args["msg"].(string)
					r := handleEcho(EchoReq{Msg: msg})
					return map[string]interface{}{"msg": r.Msg}, nil
				},
			},
			"getUser": &graphql.Field{
				Type: userType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id, _ := p.Args["id"].(int)
					u := handleGetUser(UserReq{ID: id})
					return map[string]interface{}{
						"id": u.ID, "name": u.Name,
						"email": u.Email, "country": u.Country, "score": u.Score,
					}, nil
				},
			},
		},
	})

	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"createOrder": &graphql.Field{
				Type: orderType,
				Args: graphql.FieldConfigArgument{
					"userId":    &graphql.ArgumentConfig{Type: graphql.Int},
					"productId": &graphql.ArgumentConfig{Type: graphql.Int},
					"qty":       &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					userID, _ := p.Args["userId"].(int)
					productID, _ := p.Args["productId"].(int)
					qty, _ := p.Args["qty"].(int)
					o := handleCreateOrder(OrderReq{UserID: userID, ProductID: productID, Qty: qty})
					return map[string]interface{}{"id": o.ID, "status": o.Status, "total": o.Total}, nil
				},
			},
		},
	})

	var err error
	gqlSchema, err = graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
	if err != nil {
		panic(err)
	}
}

type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func graphqlHandler(w http.ResponseWriter, r *http.Request) {
	var req gqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result := graphql.Do(graphql.Params{
		Schema:         gqlSchema,
		RequestString:  req.Query,
		VariableValues: req.Variables,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
