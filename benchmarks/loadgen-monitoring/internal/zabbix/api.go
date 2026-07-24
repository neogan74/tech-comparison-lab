package zabbix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// API is a minimal Zabbix JSON-RPC client. It targets the api_jsonrpc.php
// endpoint served by the Zabbix frontend and authenticates with a session
// token passed as an HTTP Bearer header (Zabbix 6.4+).
type API struct {
	url   string
	http  *http.Client
	token string
	id    atomic.Int64
}

// NewAPI creates a client for the given api_jsonrpc.php URL
// (e.g. http://localhost:8090/api_jsonrpc.php).
func NewAPI(url string) *API {
	return &API{url: url, http: &http.Client{Timeout: 60 * time.Second}}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      int64  `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("zabbix rpc error %d: %s (%s)", e.Code, e.Message, e.Data)
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

// call issues one JSON-RPC request and unmarshals result into out (which may
// be nil to discard the result). authed controls whether the Bearer token is
// attached.
func (a *API) call(ctx context.Context, method string, params any, authed bool, out any) error {
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      a.id.Add(1),
	})
	if err != nil {
		return fmt.Errorf("marshal %s: %w", method, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json-rpc")
	if authed && a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("do %s: %w", method, err)
	}
	defer resp.Body.Close()

	var r rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if r.Error != nil {
		return r.Error
	}
	if out != nil {
		if err := json.Unmarshal(r.Result, out); err != nil {
			return fmt.Errorf("unmarshal %s result: %w", method, err)
		}
	}
	return nil
}

// Version returns the API version string (no authentication required); it
// doubles as a connectivity check.
func (a *API) Version(ctx context.Context) (string, error) {
	var v string
	// params must be an empty object, not null, for apiinfo.version.
	if err := a.call(ctx, "apiinfo.version", struct{}{}, false, &v); err != nil {
		return "", err
	}
	return v, nil
}

// Login authenticates with username/password and stores the session token for
// subsequent authenticated calls.
func (a *API) Login(ctx context.Context, user, password string) error {
	var token string
	params := map[string]string{"username": user, "password": password}
	if err := a.call(ctx, "user.login", params, false, &token); err != nil {
		return err
	}
	a.token = token
	return nil
}

// EnsureHostGroup returns the id of host group name, creating it if absent.
func (a *API) EnsureHostGroup(ctx context.Context, name string) (string, error) {
	var existing []struct {
		GroupID string `json:"groupid"`
	}
	getParams := map[string]any{
		"output": []string{"groupid"},
		"filter": map[string]any{"name": []string{name}},
	}
	if err := a.call(ctx, "hostgroup.get", getParams, true, &existing); err != nil {
		return "", err
	}
	if len(existing) > 0 {
		return existing[0].GroupID, nil
	}

	var created struct {
		GroupIDs []string `json:"groupids"`
	}
	if err := a.call(ctx, "hostgroup.create", map[string]any{"name": name}, true, &created); err != nil {
		return "", err
	}
	if len(created.GroupIDs) == 0 {
		return "", fmt.Errorf("hostgroup.create returned no id")
	}
	return created.GroupIDs[0], nil
}

// EnsureHost returns the id of host, creating it in groupID if absent.
func (a *API) EnsureHost(ctx context.Context, host, groupID string) (string, error) {
	var existing []struct {
		HostID string `json:"hostid"`
	}
	getParams := map[string]any{
		"output": []string{"hostid"},
		"filter": map[string]any{"host": []string{host}},
	}
	if err := a.call(ctx, "host.get", getParams, true, &existing); err != nil {
		return "", err
	}
	if len(existing) > 0 {
		return existing[0].HostID, nil
	}

	var created struct {
		HostIDs []string `json:"hostids"`
	}
	createParams := map[string]any{
		"host":   host,
		"groups": []map[string]string{{"groupid": groupID}},
	}
	if err := a.call(ctx, "host.create", createParams, true, &created); err != nil {
		return "", err
	}
	if len(created.HostIDs) == 0 {
		return "", fmt.Errorf("host.create returned no id")
	}
	return created.HostIDs[0], nil
}

// itemSpec describes one trapper item to create.
type itemSpec struct {
	Name string
	Key  string
}

// CreateItems creates the given trapper items on hostID in a single
// item.create call and returns the resulting itemids, in input order.
func (a *API) CreateItems(ctx context.Context, hostID string, specs []itemSpec) ([]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	params := make([]map[string]any, len(specs))
	for i, s := range specs {
		params[i] = map[string]any{
			"hostid":     hostID,
			"name":       s.Name,
			"key_":       s.Key,
			"type":       2, // Zabbix trapper
			"value_type": 0, // numeric float
		}
	}
	var created struct {
		ItemIDs []string `json:"itemids"`
	}
	if err := a.call(ctx, "item.create", params, true, &created); err != nil {
		return nil, err
	}
	if len(created.ItemIDs) != len(specs) {
		return nil, fmt.Errorf("item.create returned %d ids, want %d", len(created.ItemIDs), len(specs))
	}
	return created.ItemIDs, nil
}

// LatestValues fetches lastvalue for every item on hostID. When nameSearch is
// non-empty, only items whose name contains it are returned. It returns the
// number of items in the response (used as a sanity check, not timed data).
func (a *API) LatestValues(ctx context.Context, hostID, nameSearch string) (int, error) {
	params := map[string]any{
		"output":  []string{"itemid", "lastvalue"},
		"hostids": []string{hostID},
	}
	if nameSearch != "" {
		params["search"] = map[string]string{"name": nameSearch}
	}
	var items []json.RawMessage
	if err := a.call(ctx, "item.get", params, true, &items); err != nil {
		return 0, err
	}
	return len(items), nil
}

// History fetches numeric-float history rows for itemIDs within the last
// windowSec seconds. It returns the number of rows (sanity check only).
func (a *API) History(ctx context.Context, itemIDs []string, windowSec int64) (int, error) {
	now := time.Now().Unix()
	params := map[string]any{
		"output":    "extend",
		"history":   0, // numeric float
		"itemids":   itemIDs,
		"time_from": now - windowSec,
		"time_till": now,
		"sortfield": "clock",
		"sortorder": "ASC",
	}
	var rows []json.RawMessage
	if err := a.call(ctx, "history.get", params, true, &rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}
