package bbapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type Runtime struct {
	DisplayStatus string `json:"displayStatus"`
}

type Thread struct {
	ID                    string  `json:"id"`
	ProjectID             string  `json:"projectId"`
	Title                 *string `json:"title"`
	TitleFallback         *string `json:"titleFallback"`
	Status                string  `json:"status"`
	Runtime               Runtime `json:"runtime"`
	QueuedWork            string  `json:"queuedWork"`
	HasPendingInteraction bool    `json:"hasPendingInteraction"`
	PinnedAt              *int64  `json:"pinnedAt"`
	ArchivedAt            *int64  `json:"archivedAt"`
	CreatedAt             int64   `json:"createdAt"`
	UpdatedAt             int64   `json:"updatedAt"`
	EnvironmentBranchName *string `json:"environmentBranchName"`
	EnvironmentPath       *string `json:"environmentPath"`
	ProviderID            string  `json:"providerId"`
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type runtimeFile struct {
	ServerURL string `json:"serverUrl"`
}

func DefaultBaseURL() string {
	if fromEnv := strings.TrimSpace(os.Getenv("BB_SERVER_URL")); fromEnv != "" {
		return strings.TrimRight(fromEnv, "/")
	}
	home, err := os.UserHomeDir()
	if err == nil {
		path := filepath.Join(home, ".bb", "bb-app-runtime.json")
		if raw, readErr := os.ReadFile(path); readErr == nil {
			var parsed runtimeFile
			if json.Unmarshal(raw, &parsed) == nil && parsed.ServerURL != "" {
				return strings.TrimRight(parsed.ServerURL, "/")
			}
		}
	}
	return "http://127.0.0.1:38886"
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL()
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(path string, query url.Values, out any) error {
	endpoint := c.BaseURL + "/api/v1" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("bb server unreachable at %s: %w", c.BaseURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("bb %s returned %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(out)
}

func (c *Client) ListThreads() ([]Thread, error) {
	var threads []Thread
	if err := c.get("/threads", nil, &threads); err != nil {
		return nil, err
	}
	return threads, nil
}

func (c *Client) ListProjects() ([]Project, error) {
	var projects []Project
	query := url.Values{}
	query.Set("includePersonal", "true")
	if err := c.get("/projects", query, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) Health() error {
	request, err := http.NewRequest(http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("bb server unreachable at %s: %w", c.BaseURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bb server at %s returned %d", c.BaseURL, response.StatusCode)
	}
	return nil
}

type TimelineRow struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Role      string  `json:"role"`
	Text      string  `json:"text"`
	Title     string  `json:"title"`
	WorkKind  string  `json:"workKind"`
	Status    string  `json:"status"`
	Command   string  `json:"command"`
	Path      string  `json:"path"`
	Query     string  `json:"query"`
	ExitCode  *int    `json:"exitCode"`
	Question  string  `json:"question"`
	CreatedAt int64   `json:"createdAt"`
	StartedAt int64   `json:"startedAt"`
	Summary   *string `json:"summary"`
}

type Timeline struct {
	MaxSeq int64         `json:"maxSeq"`
	Rows   []TimelineRow `json:"rows"`
}

func (c *Client) Timeline(threadID string, segmentLimit int) (*Timeline, error) {
	query := url.Values{}
	if segmentLimit > 0 {
		query.Set("segmentLimit", fmt.Sprintf("%d", segmentLimit))
	}
	var timeline Timeline
	if err := c.get("/threads/"+threadID+"/timeline", query, &timeline); err != nil {
		return nil, err
	}
	return &timeline, nil
}
