package executor

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

	"github.com/kaii9/codingJudge/internal/judge"
)

type Client struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

func NewClient(endpoint, token string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("executor endpoint must be an absolute http or https URL")
	}
	if token == "" {
		return nil, fmt.Errorf("executor token is required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("executor timeout must be positive")
	}
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Run(ctx context.Context, req judge.RunRequest) (judge.RunResult, error) {
	results, err := c.RunBatch(ctx, req, []string{req.Input})
	if err != nil {
		return judge.RunResult{}, err
	}
	if len(results) == 0 {
		return judge.RunResult{}, fmt.Errorf("executor returned no result")
	}
	return results[0], nil
}

func (c *Client) RunBatch(ctx context.Context, req judge.RunRequest, inputs []string) ([]judge.RunResult, error) {
	payload, err := json.Marshal(requestFrom(req, inputs))
	if err != nil {
		return nil, fmt.Errorf("encode executor request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/run-batch", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create executor request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call executor: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("executor returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var decoded batchResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode executor response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode executor response: trailing data")
	}
	if len(decoded.Results) == 0 || len(decoded.Results) > len(inputs) {
		return nil, fmt.Errorf("executor returned invalid result count %d for %d inputs", len(decoded.Results), len(inputs))
	}
	return decoded.Results, nil
}

var _ judge.Runner = (*Client)(nil)
var _ judge.BatchRunner = (*Client)(nil)
