package executor

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/kaii9/codingJudge/internal/judge"
)

type ClientPool struct {
	clients []*Client
	next    atomic.Uint64
}

func NewClientPool(endpoints []string, token string, timeout time.Duration) (*ClientPool, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("at least one executor endpoint is required")
	}
	pool := &ClientPool{clients: make([]*Client, 0, len(endpoints))}
	for index, endpoint := range endpoints {
		client, err := NewClient(endpoint, token, timeout)
		if err != nil {
			return nil, fmt.Errorf("executor endpoint %d: %w", index+1, err)
		}
		pool.clients = append(pool.clients, client)
	}
	return pool, nil
}

func (p *ClientPool) Run(ctx context.Context, request judge.RunRequest) (judge.RunResult, error) {
	return p.client().Run(ctx, request)
}

func (p *ClientPool) RunBatch(ctx context.Context, request judge.RunRequest, inputs []string) ([]judge.RunResult, error) {
	return p.client().RunBatch(ctx, request, inputs)
}

func (p *ClientPool) client() *Client {
	index := p.next.Add(1) - 1
	return p.clients[index%uint64(len(p.clients))]
}

var _ judge.Runner = (*ClientPool)(nil)
var _ judge.BatchRunner = (*ClientPool)(nil)
