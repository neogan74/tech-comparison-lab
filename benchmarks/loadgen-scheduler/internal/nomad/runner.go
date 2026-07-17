package nomad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const workloadName = "scheduler-bench"

type Runner struct {
	address  string
	client   *http.Client
	replicas int
}

type allocation struct {
	ID            string `json:"ID"`
	ClientStatus  string `json:"ClientStatus"`
	DesiredStatus string `json:"DesiredStatus"`
}

func New(address string) (*Runner, error) {
	address = strings.TrimRight(address, "/")
	if _, err := url.ParseRequestURI(address); err != nil {
		return nil, fmt.Errorf("invalid Nomad address: %w", err)
	}
	return newWithClient(address, &http.Client{Timeout: 30 * time.Second})
}

func newWithClient(address string, client *http.Client) (*Runner, error) {
	runner := &Runner{address: address, client: client}
	var leader string
	if err := runner.request(context.Background(), http.MethodGet, "/v1/status/leader", nil, &leader); err != nil {
		return nil, fmt.Errorf("Nomad health check: %w", err)
	}
	if leader == "" {
		return nil, fmt.Errorf("Nomad health check: cluster has no leader")
	}
	return runner, nil
}

func (runner *Runner) Name() string { return "nomad" }

func (runner *Runner) Cleanup(ctx context.Context) error {
	err := runner.request(ctx, http.MethodDelete, "/v1/job/"+workloadName+"?purge=true", nil, nil)
	if err != nil && !strings.Contains(err.Error(), "status 404") {
		return err
	}
	runner.replicas = 0
	return runner.waitRunning(ctx, 0, "")
}

func (runner *Runner) Deploy(ctx context.Context, replicas int) (time.Duration, error) {
	start := time.Now()
	if err := runner.register(ctx, replicas); err != nil {
		return 0, err
	}
	runner.replicas = replicas
	if err := runner.waitRunning(ctx, replicas, ""); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) Scale(ctx context.Context, replicas int) (time.Duration, error) {
	start := time.Now()
	if err := runner.register(ctx, replicas); err != nil {
		return 0, err
	}
	runner.replicas = replicas
	if err := runner.waitRunning(ctx, replicas, ""); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) Recover(ctx context.Context) (time.Duration, error) {
	allocations, err := runner.allocations(ctx)
	if err != nil {
		return 0, err
	}
	victim := ""
	for _, value := range allocations {
		if value.ClientStatus == "running" && value.DesiredStatus == "run" {
			victim = value.ID
			break
		}
	}
	if victim == "" {
		return 0, fmt.Errorf("no running Nomad allocation available for recovery benchmark")
	}
	start := time.Now()
	if err := runner.request(ctx, http.MethodPost, "/v1/allocation/"+victim+"/stop", map[string]any{}, nil); err != nil {
		return 0, err
	}
	if err := runner.waitRunning(ctx, runner.replicas, victim); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) register(ctx context.Context, replicas int) error {
	job := map[string]any{
		"ID": workloadName, "Name": workloadName, "Type": "service", "Datacenters": []string{"dc1"},
		"TaskGroups": []any{map[string]any{
			"Name": "bench", "Count": replicas,
			"RestartPolicy": map[string]any{"Attempts": 0, "Mode": "fail"},
			"Tasks": []any{map[string]any{
				"Name": "bench", "Driver": "raw_exec",
				"Config": map[string]any{
					"command": "/bin/sh",
					"args":    []string{"-c", "while true; do sleep 60; done"},
				},
				"Resources": map[string]any{"CPU": 10, "MemoryMB": 10},
			}},
		}},
	}
	return runner.request(ctx, http.MethodPost, "/v1/jobs", map[string]any{"Job": job}, nil)
}

func (runner *Runner) allocations(ctx context.Context) ([]allocation, error) {
	var values []allocation
	err := runner.request(ctx, http.MethodGet, "/v1/job/"+workloadName+"/allocations", nil, &values)
	if err != nil && strings.Contains(err.Error(), "status 404") {
		return nil, nil
	}
	return values, err
}

func (runner *Runner) waitRunning(ctx context.Context, expected int, excludedID string) error {
	for {
		allocations, err := runner.allocations(ctx)
		if err != nil {
			return err
		}
		running := 0
		excludedRunning := false
		for _, value := range allocations {
			if value.ClientStatus == "running" && value.DesiredStatus == "run" {
				running++
				if value.ID == excludedID {
					excludedRunning = true
				}
			}
		}
		if running == expected && !excludedRunning {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %d running Nomad allocations: %w", expected, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (runner *Runner) request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, runner.address+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := runner.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("%s %s: status %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(message)))
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}
