package gitea

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client speaks to the Gitea REST API v1.
type Client struct {
	baseURL string
	user    string
	token   string
	pass    string
	http    *http.Client
}

func NewClient(baseURL, user, token, pass string) *Client {
	return &Client{
		baseURL: baseURL,
		user:    user,
		token:   token,
		pass:    pass,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) auth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	} else if c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	c.auth(req)
	return c.http.Do(req)
}

// Ping verifies that the Gitea API is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/settings/api", nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ping: HTTP %d", resp.StatusCode)
	}
	return nil
}

// EnsureRepo creates the repository if it does not already exist.
func (c *Client) EnsureRepo(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s", c.baseURL, c.user, name)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}

	body, _ := json.Marshal(map[string]interface{}{
		"name":           name,
		"private":        false,
		"auto_init":      true,
		"default_branch": "main",
	})
	req, _ = http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/user/repos", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create repo: HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}

// getFileResp is returned by GET /repos/.../contents/<path>.
type getFileResp struct {
	SHA     string `json:"sha"`
	Content string `json:"content"` // base64-encoded file data (may include newlines)
}

// putFileResp is returned by POST/PUT /repos/.../contents/<path>.
type putFileResp struct {
	Content *struct {
		SHA string `json:"sha"`
	} `json:"content"`
}

// GetFile returns the raw content and current SHA of a file.
// Returns (nil, "", nil) when the file does not exist.
func (c *Client) GetFile(ctx context.Context, repo, path string) ([]byte, string, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/%s", c.baseURL, c.user, repo, path)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, "", nil
	}
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("get file %s: HTTP %d", path, resp.StatusCode)
	}
	var fr getFileResp
	if err := json.NewDecoder(resp.Body).Decode(&fr); err != nil {
		return nil, "", err
	}
	// Gitea wraps lines in base64 with newlines; StdEncoding handles that.
	raw, err := base64.StdEncoding.DecodeString(fr.Content)
	return raw, fr.SHA, err
}

// PutFile creates or updates a file and returns the new SHA.
func (c *Client) PutFile(ctx context.Context, repo, path, message string, content []byte, sha string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/%s", c.baseURL, c.user, repo, path)
	payload := map[string]interface{}{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  "main",
	}
	method := "POST"
	if sha != "" {
		payload["sha"] = sha
		method = "PUT"
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("put file %s: HTTP %d: %s", path, resp.StatusCode, b)
	}
	var pr putFileResp
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", err
	}
	if pr.Content != nil {
		return pr.Content.SHA, nil
	}
	return "", nil
}

// DeleteFile removes a file from the repository.
func (c *Client) DeleteFile(ctx context.Context, repo, path, sha string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/contents/%s", c.baseURL, c.user, repo, path)
	payload := map[string]interface{}{
		"message": "cleanup: " + path,
		"sha":     sha,
		"branch":  "main",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "DELETE", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete file %s: HTTP %d", path, resp.StatusCode)
	}
	return nil
}
