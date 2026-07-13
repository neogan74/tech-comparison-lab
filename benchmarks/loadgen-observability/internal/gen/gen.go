// Package gen produces synthetic Prometheus time series for the benchmark.
package gen

import (
	"fmt"
	"math"
	mrand "math/rand"
)

var (
	regions  = []string{"us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1"}
	services = []string{"api", "worker", "cache", "db", "gateway", "auth", "billing", "search", "queue", "web"}
)

// Series describes one unique metric time series (a fixed label set).
type Series struct {
	Labels []Label
}

// Label is a Prometheus label name/value pair.
type Label struct {
	Name  string
	Value string
}

// Series builds n deterministic series for the given metric name.
// Cardinality is exactly n: every series carries a unique "instance" label.
func Build(metric string, n int) []Series {
	out := make([]Series, n)
	for i := 0; i < n; i++ {
		out[i] = Series{Labels: []Label{
			{Name: "__name__", Value: metric},
			{Name: "job", Value: "bench"},
			{Name: "region", Value: regions[i%len(regions)]},
			{Name: "service", Value: services[(i/len(regions))%len(services)]},
			{Name: "instance", Value: fmt.Sprintf("bench-%d", i)},
		}}
	}
	return out
}

// Value returns a deterministic-ish synthetic gauge value (sine wave + noise)
// for series index i at sample index t, so repeated runs produce similar shapes.
func Value(i, t int, rnd *mrand.Rand) float64 {
	base := 50 + 40*math.Sin(float64(t)/12.0+float64(i%17))
	noise := rnd.Float64()*5 - 2.5
	return base + noise
}
