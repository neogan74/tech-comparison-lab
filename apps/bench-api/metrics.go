package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bench_api",
			Name:      "requests_total",
			Help:      "Total handled requests by protocol, operation, and status.",
		},
		[]string{"protocol", "operation", "status"},
	)

	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "bench_api",
			Name:      "request_duration_seconds",
			Help:      "Request duration by protocol and operation.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"protocol", "operation"},
	)

	inFlightRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "bench_api",
			Name:      "in_flight_requests",
			Help:      "Current in-flight requests by protocol and operation.",
		},
		[]string{"protocol", "operation"},
	)
)

func metricsHandler() http.Handler {
	return promhttp.Handler()
}

func instrumentREST(operation string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		inFlightRequests.WithLabelValues("rest", operation).Inc()
		defer inFlightRequests.WithLabelValues("rest", operation).Dec()

		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next(rec, r)

		statusText := http.StatusText(rec.statusCode)
		if statusText == "" {
			statusText = "UNKNOWN"
		}
		statusText = strings.ToLower(statusText)

		requestsTotal.WithLabelValues("rest", operation, statusText).Inc()
		requestDuration.WithLabelValues("rest", operation).Observe(time.Since(start).Seconds())
	}
}

func grpcMetricsInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		op := grpcOperation(info.FullMethod)
		start := time.Now()
		inFlightRequests.WithLabelValues("grpc", op).Inc()
		defer inFlightRequests.WithLabelValues("grpc", op).Dec()

		resp, err := handler(ctx, req)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		requestsTotal.WithLabelValues("grpc", op, strings.ToLower(code.String())).Inc()
		requestDuration.WithLabelValues("grpc", op).Observe(time.Since(start).Seconds())
		return resp, err
	}
}

func grpcOperation(fullMethod string) string {
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	parts := strings.Split(fullMethod, "/")
	if len(parts) == 0 {
		return "unknown"
	}
	switch strings.ToLower(parts[len(parts)-1]) {
	case "getuser":
		return "get-user"
	case "createorder":
		return "create-order"
	default:
		return strings.ToLower(parts[len(parts)-1])
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}
