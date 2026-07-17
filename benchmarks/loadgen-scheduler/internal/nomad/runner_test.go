package nomad

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRunnerLifecycle(t *testing.T) {
	var mutex sync.Mutex
	replicas := 0
	generation := 0
	driver := ""
	memoryMB := 0

	allocations := func() []allocation {
		values := make([]allocation, 0, replicas)
		for index := 0; index < replicas; index++ {
			values = append(values, allocation{
				ID: fmt.Sprintf("alloc-%d-%d", generation, index), ClientStatus: "running", DesiredStatus: "run",
			})
		}
		return values
	}

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/status/leader":
			_ = json.NewEncoder(writer).Encode("127.0.0.1:4647")
		case request.Method == http.MethodPost && request.URL.Path == "/v1/jobs":
			var payload struct {
				Job struct {
					TaskGroups []struct {
						Count int `json:"Count"`
						Tasks []struct {
							Driver    string `json:"Driver"`
							Resources struct {
								MemoryMB int `json:"MemoryMB"`
							} `json:"Resources"`
						} `json:"Tasks"`
					} `json:"TaskGroups"`
				} `json:"Job"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Errorf("decode registration: %v", err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			replicas = payload.Job.TaskGroups[0].Count
			driver = payload.Job.TaskGroups[0].Tasks[0].Driver
			memoryMB = payload.Job.TaskGroups[0].Tasks[0].Resources.MemoryMB
			generation++
			_ = json.NewEncoder(writer).Encode(map[string]int{"EvalIndex": generation})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/job/scheduler-bench/allocations":
			_ = json.NewEncoder(writer).Encode(allocations())
		case request.Method == http.MethodPost && strings.HasPrefix(request.URL.Path, "/v1/allocation/") && strings.HasSuffix(request.URL.Path, "/stop"):
			generation++
			_ = json.NewEncoder(writer).Encode(map[string]int{"EvalIndex": generation})
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/job/scheduler-bench":
			replicas = 0
			generation++
			_ = json.NewEncoder(writer).Encode(map[string]int{"EvalIndex": generation})
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	})

	runner, err := newWithClient("http://nomad.test", clientForHandler(handler))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := runner.Deploy(ctx, 1); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if driver != "raw_exec" {
		t.Fatalf("driver = %q, want raw_exec", driver)
	}
	if memoryMB < 10 {
		t.Fatalf("MemoryMB = %d, want at least 10 for Nomad validation", memoryMB)
	}
	if _, err := runner.Scale(ctx, 3); err != nil {
		t.Fatalf("Scale: %v", err)
	}
	if _, err := runner.Recover(ctx); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if err := runner.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}

func TestNewRejectsLeaderlessNomad(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode("")
	})
	if _, err := newWithClient("http://nomad.test", clientForHandler(handler)); err == nil {
		t.Fatal("expected leaderless Nomad to be rejected")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func clientForHandler(handler http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		response := recorder.Result()
		response.Request = request
		if response.Body == nil {
			response.Body = io.NopCloser(strings.NewReader(""))
		}
		return response, nil
	})}
}
