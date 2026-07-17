package swarm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	workloadName  = "scheduler-bench"
	workloadImage = "registry.k8s.io/pause:3.10"
)

type Runner struct {
	address   string
	client    *http.Client
	serviceID string
	replicas  int
}

type service struct {
	ID      string `json:"ID"`
	Version struct {
		Index uint64 `json:"Index"`
	} `json:"Version"`
	Spec serviceSpec `json:"Spec"`
}

type serviceSpec struct {
	Name         string       `json:"Name"`
	TaskTemplate taskTemplate `json:"TaskTemplate"`
	Mode         serviceMode  `json:"Mode"`
}

type taskTemplate struct {
	ContainerSpec containerSpec `json:"ContainerSpec"`
	RestartPolicy restartPolicy `json:"RestartPolicy"`
	Resources     resources     `json:"Resources"`
	ForceUpdate   uint64        `json:"ForceUpdate,omitempty"`
}

type containerSpec struct {
	Image string `json:"Image"`
}

type restartPolicy struct {
	Condition string `json:"Condition"`
}

type resources struct {
	Reservations resourceLimit `json:"Reservations"`
}

type resourceLimit struct {
	NanoCPUs    int64 `json:"NanoCPUs"`
	MemoryBytes int64 `json:"MemoryBytes"`
}

type serviceMode struct {
	Replicated replicatedMode `json:"Replicated"`
}

type replicatedMode struct {
	Replicas int `json:"Replicas"`
}

type task struct {
	ID           string `json:"ID"`
	DesiredState string `json:"DesiredState"`
	Status       struct {
		State           string `json:"State"`
		ContainerStatus struct {
			ContainerID string `json:"ContainerID"`
		} `json:"ContainerStatus"`
	} `json:"Status"`
}

func New(host string) (*Runner, error) {
	address, client, err := clientForHost(host)
	if err != nil {
		return nil, err
	}
	return newWithClient(address, client)
}

func clientForHost(host string) (string, *http.Client, error) {
	parsed, err := url.Parse(host)
	if err != nil {
		return "", nil, fmt.Errorf("invalid Docker host: %w", err)
	}
	switch parsed.Scheme {
	case "unix":
		if parsed.Path == "" {
			return "", nil, fmt.Errorf("invalid Docker host: unix socket path is empty")
		}
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", parsed.Path)
		}}
		return "http://docker", &http.Client{Transport: transport, Timeout: 30 * time.Second}, nil
	case "http", "https":
		return strings.TrimRight(host, "/"), &http.Client{Timeout: 30 * time.Second}, nil
	default:
		return "", nil, fmt.Errorf("invalid Docker host: scheme must be unix, http, or https")
	}
}

func newWithClient(address string, client *http.Client) (*Runner, error) {
	runner := &Runner{address: strings.TrimRight(address, "/"), client: client}
	if err := runner.request(context.Background(), http.MethodGet, "/_ping", nil, nil); err != nil {
		return nil, fmt.Errorf("Docker health check: %w", err)
	}
	var info struct {
		Swarm struct {
			LocalNodeState string `json:"LocalNodeState"`
		} `json:"Swarm"`
	}
	if err := runner.request(context.Background(), http.MethodGet, "/info", nil, &info); err != nil {
		return nil, fmt.Errorf("Docker info: %w", err)
	}
	if info.Swarm.LocalNodeState != "active" {
		return nil, fmt.Errorf("Docker Swarm is not active")
	}
	return runner, nil
}

func (runner *Runner) Name() string { return "swarm" }

func (runner *Runner) Cleanup(ctx context.Context) error {
	if runner.serviceID == "" {
		values, err := runner.services(ctx)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			return nil
		}
		runner.serviceID = values[0].ID
	}
	serviceID := runner.serviceID
	err := runner.request(ctx, http.MethodDelete, "/services/"+serviceID, nil, nil)
	if err != nil && !strings.Contains(err.Error(), "status 404") {
		return err
	}
	runner.serviceID = ""
	runner.replicas = 0
	return runner.waitRunning(ctx, serviceID, 0, "")
}

func (runner *Runner) Deploy(ctx context.Context, replicas int) (time.Duration, error) {
	start := time.Now()
	var response struct {
		ID string `json:"ID"`
	}
	if err := runner.request(ctx, http.MethodPost, "/services/create", newServiceSpec(replicas), &response); err != nil {
		return 0, err
	}
	runner.serviceID = response.ID
	runner.replicas = replicas
	if err := runner.waitRunning(ctx, runner.serviceID, replicas, ""); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) Scale(ctx context.Context, replicas int) (time.Duration, error) {
	value, err := runner.inspect(ctx)
	if err != nil {
		return 0, err
	}
	value.Spec.Mode.Replicated.Replicas = replicas
	start := time.Now()
	path := fmt.Sprintf("/services/%s/update?version=%d", runner.serviceID, value.Version.Index)
	if err := runner.request(ctx, http.MethodPost, path, value.Spec, nil); err != nil {
		return 0, err
	}
	runner.replicas = replicas
	if err := runner.waitRunning(ctx, runner.serviceID, replicas, ""); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) Recover(ctx context.Context) (time.Duration, error) {
	values, err := runner.tasks(ctx, runner.serviceID)
	if err != nil {
		return 0, err
	}
	var victim task
	for _, value := range values {
		if value.DesiredState == "running" && value.Status.State == "running" && value.Status.ContainerStatus.ContainerID != "" {
			victim = value
			break
		}
	}
	if victim.ID == "" {
		return 0, fmt.Errorf("no running Docker Swarm task available for recovery benchmark")
	}
	start := time.Now()
	path := "/containers/" + victim.Status.ContainerStatus.ContainerID + "/kill?signal=KILL"
	if err := runner.request(ctx, http.MethodPost, path, nil, nil); err != nil {
		return 0, err
	}
	if err := runner.waitRunning(ctx, runner.serviceID, runner.replicas, victim.ID); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func newServiceSpec(replicas int) serviceSpec {
	return serviceSpec{
		Name: workloadName,
		TaskTemplate: taskTemplate{
			ContainerSpec: containerSpec{Image: workloadImage},
			RestartPolicy: restartPolicy{Condition: "any"},
			Resources:     resources{Reservations: resourceLimit{NanoCPUs: 10_000_000, MemoryBytes: 8 * 1024 * 1024}},
		},
		Mode: serviceMode{Replicated: replicatedMode{Replicas: replicas}},
	}
}

func (runner *Runner) inspect(ctx context.Context) (service, error) {
	var value service
	err := runner.request(ctx, http.MethodGet, "/services/"+runner.serviceID, nil, &value)
	return value, err
}

func (runner *Runner) services(ctx context.Context) ([]service, error) {
	filters, _ := json.Marshal(map[string][]string{"name": {workloadName}})
	var values []service
	err := runner.request(ctx, http.MethodGet, "/services?filters="+url.QueryEscape(string(filters)), nil, &values)
	return values, err
}

func (runner *Runner) tasks(ctx context.Context, serviceID string) ([]task, error) {
	filters, _ := json.Marshal(map[string][]string{"service": {serviceID}})
	var values []task
	err := runner.request(ctx, http.MethodGet, "/tasks?filters="+url.QueryEscape(string(filters)), nil, &values)
	if err != nil && strings.Contains(err.Error(), "status 404") {
		return nil, nil
	}
	return values, err
}

func (runner *Runner) waitRunning(ctx context.Context, serviceID string, expected int, excludedID string) error {
	for {
		values, err := runner.tasks(ctx, serviceID)
		if err != nil {
			return err
		}
		running := 0
		excludedRunning := false
		for _, value := range values {
			if value.DesiredState == "running" && value.Status.State == "running" {
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
			return fmt.Errorf("wait for %d running Docker Swarm tasks: %w", expected, ctx.Err())
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
