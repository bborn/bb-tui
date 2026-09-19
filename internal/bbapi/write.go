package bbapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Client) post(path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1"+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("bb server unreachable at %s: %w", c.BaseURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("bb %s returned %d: %s", path, response.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}

type promptText struct {
	Type     string   `json:"type"`
	Text     string   `json:"text"`
	Mentions []string `json:"mentions"`
}

type sendRequest struct {
	Input          []any  `json:"input"`
	Mode           string `json:"mode"`
	Model          string `json:"model,omitempty"`
	PermissionMode string `json:"permissionMode,omitempty"`
}

// LocalAttachment is a file on this machine to send with a message. bb reads it
// server-side, so only the path travels.
type LocalAttachment struct {
	Path  string
	Image bool
}

// Send posts a message to a thread. "auto" lets bb decide between starting a
// turn and steering one already in flight, which is what the app's composer
// does.
func (c *Client) Send(threadID, text string, attachments []LocalAttachment, model, permission string) error {
	input := []any{}
	if strings.TrimSpace(text) != "" {
		input = append(input, promptText{Type: "text", Text: text, Mentions: []string{}})
	}
	for _, attachment := range attachments {
		kind := "localFile"
		if attachment.Image {
			kind = "localImage"
		}
		input = append(input, map[string]any{"type": kind, "path": attachment.Path})
	}
	if len(input) == 0 {
		return nil
	}
	return c.post("/threads/"+threadID+"/send", sendRequest{
		Input:          input,
		Mode:           "auto",
		Model:          model,
		PermissionMode: permission,
	}, nil)
}

func (c *Client) Stop(threadID string) error {
	return c.post("/threads/"+threadID+"/stop", struct{}{}, nil)
}

func (c *Client) Archive(threadID string) error {
	return c.post("/threads/"+threadID+"/archive-all", struct{}{}, nil)
}

func (c *Client) Pin(threadID string) error {
	return c.post("/threads/"+threadID+"/pin", struct{}{}, nil)
}

func (c *Client) Unpin(threadID string) error {
	return c.post("/threads/"+threadID+"/unpin", struct{}{}, nil)
}

type QuestionOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Question struct {
	ID          string           `json:"id"`
	Prompt      string           `json:"prompt"`
	ShortLabel  string           `json:"shortLabel"`
	MultiSelect bool             `json:"multiSelect"`
	Options     []QuestionOption `json:"options"`
}

type InteractionPayload struct {
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Command   string     `json:"command"`
	Path      string     `json:"path"`
	ToolName  string     `json:"toolName"`
	Questions []Question `json:"questions"`
}

type Interaction struct {
	ID        string             `json:"id"`
	ThreadID  string             `json:"threadId"`
	Status    string             `json:"status"`
	CreatedAt int64              `json:"createdAt"`
	Payload   InteractionPayload `json:"payload"`
}

// Summary is the one-line description shown in a picker.
func (i Interaction) Summary() string {
	payload := i.Payload
	switch payload.Kind {
	case "approval":
		for _, candidate := range []string{payload.Title, payload.Command, payload.Path, payload.ToolName} {
			if strings.TrimSpace(candidate) != "" {
				return collapseSpaces(candidate)
			}
		}
		return "approval requested"
	case "user_question":
		if len(payload.Questions) > 0 {
			return collapseSpaces(payload.Questions[0].Prompt)
		}
		return "a question"
	}
	if strings.TrimSpace(payload.Title) != "" {
		return collapseSpaces(payload.Title)
	}
	return payload.Kind
}

func collapseSpaces(value string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(value), " "))
}

func (c *Client) Interactions(threadID string) ([]Interaction, error) {
	var interactions []Interaction
	if err := c.get("/threads/"+threadID+"/interactions", nil, &interactions); err != nil {
		return nil, err
	}
	pending := interactions[:0]
	for _, interaction := range interactions {
		if interaction.Status == "pending" {
			pending = append(pending, interaction)
		}
	}
	return pending, nil
}

type approvalResolution struct {
	Decision           string `json:"decision"`
	GrantedPermissions any    `json:"grantedPermissions,omitempty"`
}

// ResolveApproval answers a tool approval. Decision is one of allow_once,
// allow_for_session, deny.
func (c *Client) ResolveApproval(threadID, interactionID, decision string) error {
	body := map[string]any{"decision": decision}
	if decision != "deny" {
		body["grantedPermissions"] = nil
	}
	return c.post("/threads/"+threadID+"/interactions/"+interactionID+"/resolve", body, nil)
}

// AnswerQuestion answers a user_question. Answers map question id to the option
// values chosen for it.
func (c *Client) AnswerQuestion(threadID, interactionID string, answers map[string][]string) error {
	payload := map[string]any{}
	for questionID, selected := range answers {
		payload[questionID] = map[string]any{"selected": selected}
	}
	return c.post(
		"/threads/"+threadID+"/interactions/"+interactionID+"/resolve",
		map[string]any{"kind": "user_answer", "answers": payload},
		nil,
	)
}

func (c *Client) Delete(threadID string) error {
	request, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/api/v1/threads/"+threadID, nil)
	if err != nil {
		return err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("bb server unreachable at %s: %w", c.BaseURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("bb delete thread returned %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

type SpawnRequest struct {
	ProjectID      string
	Title          string
	Prompt         string
	ProviderID     string
	Model          string
	PermissionMode string
}

type SpawnResult struct {
	ID string `json:"id"`
}

// Spawn creates a thread and dispatches its first message. The environment is
// left as the project default, which is what the app's new-thread composer does
// unless the user picks otherwise.
func (c *Client) Spawn(request SpawnRequest) (string, error) {
	body := map[string]any{
		"projectId":   request.ProjectID,
		"origin":      "cli",
		"input":       []promptText{{Type: "text", Text: request.Prompt, Mentions: []string{}}},
		"environment": map[string]any{"type": "project-default"},
	}
	if strings.TrimSpace(request.Title) != "" {
		body["title"] = request.Title
	}
	if strings.TrimSpace(request.ProviderID) != "" {
		body["providerId"] = request.ProviderID
	}
	if strings.TrimSpace(request.Model) != "" {
		body["model"] = request.Model
	}
	if strings.TrimSpace(request.PermissionMode) != "" {
		body["permissionMode"] = request.PermissionMode
	}

	var result SpawnResult
	if err := c.post("/threads", body, &result); err != nil {
		return "", err
	}
	return result.ID, nil
}

type ExecutionOptions struct {
	Model          string `json:"model"`
	PermissionMode string `json:"permissionMode"`
	ReasoningLevel string `json:"reasoningLevel"`
	ServiceTier    string `json:"serviceTier"`
}

func (c *Client) ExecutionOptions(threadID string) (ExecutionOptions, error) {
	var options ExecutionOptions
	err := c.get("/threads/"+threadID+"/default-execution-options", nil, &options)
	return options, err
}

// PermissionModes reports what the provider will accept, so the composer offers
// the same choices the app does rather than a guessed list.
func (c *Client) PermissionModes(providerID string) ([]string, error) {
	var payload struct {
		Providers []struct {
			ID           string `json:"id"`
			Capabilities struct {
				PermissionModes []string `json:"permissionModes"`
			} `json:"capabilities"`
		} `json:"providers"`
	}
	if err := c.get("/system/execution-options", nil, &payload); err != nil {
		return nil, err
	}
	for _, provider := range payload.Providers {
		if provider.ID == providerID {
			return provider.Capabilities.PermissionModes, nil
		}
	}
	return nil, nil
}

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
}

// Skills lists what "/" can insert. environmentID may be empty, which asks for
// the project's own skills without an environment's overlay.
func (c *Client) Skills(projectID, environmentID string) ([]Skill, error) {
	query := url.Values{}
	query.Set("environmentId", environmentID)
	var payload struct {
		Skills []Skill `json:"skills"`
	}
	if err := c.get("/projects/"+projectID+"/skills", query, &payload); err != nil {
		return nil, err
	}
	return payload.Skills, nil
}

type ProjectFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// Files searches the project's tree for "@" mentions. The query is applied by
// the server, so a repo with a hundred thousand files costs one request.
func (c *Client) Files(projectID, query string, limit int) ([]ProjectFile, error) {
	params := url.Values{}
	if strings.TrimSpace(query) != "" {
		params.Set("query", query)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	var payload struct {
		Files []ProjectFile `json:"files"`
	}
	if err := c.get("/projects/"+projectID+"/files", params, &payload); err != nil {
		return nil, err
	}
	return payload.Files, nil
}

// Retry re-submits the thread's most recent failed turn. A null turnRequestId
// means that turn, which is the one whose failure put the thread in error.
func (c *Client) Retry(threadID string) error {
	return c.post("/threads/"+threadID+"/retry", map[string]any{
		"turnRequestId": nil,
	}, nil)
}

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Reasoning   []struct {
		ReasoningEffort string `json:"reasoningEffort"`
		Description     string `json:"description"`
	} `json:"supportedReasoningEfforts"`
}

type ProviderOptions struct {
	Models          []Model
	PermissionModes []string
}

// Options reports what a provider will accept for the next message: its model
// catalogue and its permission modes. Both are per-provider, so the picker
// offers exactly what the app would.
func (c *Client) Options(providerID string) (ProviderOptions, error) {
	params := url.Values{}
	if providerID != "" {
		params.Set("providerId", providerID)
	}
	var payload struct {
		Models    []Model `json:"models"`
		Providers []struct {
			ID           string `json:"id"`
			Capabilities struct {
				PermissionModes []string `json:"permissionModes"`
			} `json:"capabilities"`
		} `json:"providers"`
	}
	if err := c.get("/system/execution-options", params, &payload); err != nil {
		return ProviderOptions{}, err
	}

	options := ProviderOptions{Models: payload.Models}
	for _, provider := range payload.Providers {
		if provider.ID == providerID {
			options.PermissionModes = provider.Capabilities.PermissionModes
		}
	}
	return options, nil
}
