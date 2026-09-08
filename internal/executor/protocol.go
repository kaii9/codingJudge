package executor

import (
	"fmt"

	"github.com/kaii9/codingJudge/internal/domain"
	"github.com/kaii9/codingJudge/internal/judge"
)

const (
	maxRequestBytes  = 16 << 20
	maxResponseBytes = 256 << 20
)

type batchRequest struct {
	Language      domain.Language `json:"language"`
	Code          string          `json:"code"`
	Inputs        []string        `json:"inputs"`
	TimeLimitMS   int             `json:"timeLimitMs"`
	MemoryLimitMB int             `json:"memoryLimitMb"`
}

type batchResponse struct {
	Results []judge.RunResult `json:"results"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (r batchRequest) validate() error {
	if !domain.IsSupportedLanguage(r.Language) {
		return fmt.Errorf("unsupported language")
	}
	if r.Code == "" {
		return fmt.Errorf("code is required")
	}
	if len(r.Inputs) == 0 || len(r.Inputs) > 100 {
		return fmt.Errorf("inputs must contain between 1 and 100 cases")
	}
	if r.TimeLimitMS < 1 || r.TimeLimitMS > 60_000 {
		return fmt.Errorf("timeLimitMs must be between 1 and 60000")
	}
	if r.MemoryLimitMB < 1 || r.MemoryLimitMB > 2048 {
		return fmt.Errorf("memoryLimitMb must be between 1 and 2048")
	}
	return nil
}

func requestFrom(req judge.RunRequest, inputs []string) batchRequest {
	return batchRequest{
		Language:      req.Language,
		Code:          req.Code,
		Inputs:        inputs,
		TimeLimitMS:   req.TimeLimitMS,
		MemoryLimitMB: req.MemoryLimitMB,
	}
}

func (r batchRequest) judgeRequest() judge.RunRequest {
	return judge.RunRequest{
		Language:      r.Language,
		Code:          r.Code,
		TimeLimitMS:   r.TimeLimitMS,
		MemoryLimitMB: r.MemoryLimitMB,
	}
}
