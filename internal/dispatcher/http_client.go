package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kai/codingjudge/internal/domain"
)

type HTTPJudgeClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPJudgeClient(baseURL string, client *http.Client) *HTTPJudgeClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPJudgeClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  client,
	}
}

func (c *HTTPJudgeClient) Judge(ctx context.Context, req JudgeRequest) (domain.JudgeResult, error) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(req); err != nil {
		return domain.JudgeResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/judge", &body)
	if err != nil {
		return domain.JudgeResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return domain.JudgeResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return domain.JudgeResult{}, fmt.Errorf("worker returned %s", resp.Status)
	}

	var result domain.JudgeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return domain.JudgeResult{}, err
	}
	return result, nil
}
