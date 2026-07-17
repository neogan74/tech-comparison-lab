package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestRunnerLifecycle(t *testing.T) {
	var mutex sync.Mutex
	replicas := 0
	generation := 0
	serviceExists := false
	image := ""

	tasks := func() []task {
		values := make([]task, 0, replicas)
		for index := 0; index < replicas; index++ {
			value := task{ID: fmt.Sprintf("task-%d-%d", generation, index), DesiredState: "running"}
			value.Status.State = "running"
			value.Status.ContainerStatus.ContainerID = fmt.Sprintf("container-%d-%d", generation, index)
			values = append(values, value)
		}
		return values
	}

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/_ping":
			_, _ = writer.Write([]byte("OK"))
		case request.Method == http.MethodGet && request.URL.Path == "/info":
			_ = json.NewEncoder(writer).Encode(map[string]any{"Swarm": map[string]string{"LocalNodeState": "active"}})
		case request.Method == http.MethodGet && request.URL.Path == "/services":
			if serviceExists {
				_ = json.NewEncoder(writer).Encode([]service{{ID: "service-1"}})
			} else {
				_ = json.NewEncoder(writer).Encode([]service{})
			}
		case request.Method == http.MethodPost && request.URL.Path == "/services/create":
			var spec serviceSpec
			_ = json.NewDecoder(request.Body).Decode(&spec)
			replicas = spec.Mode.Replicated.Replicas
			image = spec.TaskTemplate.ContainerSpec.Image
			serviceExists = true
			generation++
			_ = json.NewEncoder(writer).Encode(map[string]string{"ID": "service-1"})
		case request.Method == http.MethodGet && request.URL.Path == "/services/service-1":
			value := service{ID: "service-1", Spec: newServiceSpec(replicas)}
			value.Version.Index = uint64(generation)
			_ = json.NewEncoder(writer).Encode(value)
		case request.Method == http.MethodPost && request.URL.Path == "/services/service-1/update":
			var spec serviceSpec
			_ = json.NewDecoder(request.Body).Decode(&spec)
			replicas = spec.Mode.Replicated.Replicas
			generation++
			_ = json.NewEncoder(writer).Encode(map[string]any{})
		case request.Method == http.MethodGet && request.URL.Path == "/tasks":
			_ = json.NewEncoder(writer).Encode(tasks())
		case request.Method == http.MethodPost && strings.HasPrefix(request.URL.Path, "/containers/") && strings.HasSuffix(request.URL.Path, "/kill"):
			generation++
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodDelete && request.URL.Path == "/services/service-1":
			replicas = 0
			serviceExists = false
			generation++
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	})

	runner, err := newWithClient("http://docker.test", clientForHandler(handler))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := runner.Deploy(ctx, 1); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if image != workloadImage {
		t.Fatalf("image = %q, want %q", image, workloadImage)
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

func TestNewRejectsInactiveSwarm(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/_ping" {
			_, _ = writer.Write([]byte("OK"))
			return
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"Swarm": map[string]string{"LocalNodeState": "inactive"}})
	})
	if _, err := newWithClient("http://docker.test", clientForHandler(handler)); err == nil {
		t.Fatal("expected inactive Swarm to be rejected")
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

func TestTaskFiltersAreURLSafe(t *testing.T) {
	filters, _ := json.Marshal(map[string][]string{"service": {"service/with space"}})
	encoded := url.QueryEscape(string(filters))
	if strings.Contains(encoded, " ") || strings.Contains(encoded, "/") {
		t.Fatalf("unsafe encoded filters: %s", encoded)
	}
}
