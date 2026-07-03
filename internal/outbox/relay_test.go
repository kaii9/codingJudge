package outbox_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kai/codingjudge/internal/domain"
	"github.com/kai/codingjudge/internal/outbox"
)

type fakeMetrics struct {
	mu         sync.Mutex
	results    []string
	durationsSet []bool
}

func (m *fakeMetrics) ObserveOutboxPublish(result string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = append(m.results, result)
	m.durationsSet = append(m.durationsSet, duration > 0)
}

func TestRelayRecordsMetricOnSuccess(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	st := &fakeStore{events: []domain.OutboxEvent{{ID: 7, SubmissionID: "sub-1", ClaimToken: "api-1", PublishAttempts: 1}}}
	pub := &fakePublisher{calls: &st.calls}
	metrics := &fakeMetrics{}
	relay := outbox.New(st, pub, outbox.Config{
		RelayID:       "api-1",
		BatchSize:     10,
		ClaimDuration: 30 * time.Second,
		Metrics:       metrics,
	})
	if err := relay.PublishBatch(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.results) != 1 || metrics.results[0] != "success" {
		t.Fatalf("results = %v, want [success]", metrics.results)
	}
	if len(metrics.durationsSet) != 1 || !metrics.durationsSet[0] {
		t.Fatal("expected positive duration")
	}
}

func TestRelayRecordsMetricOnError(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	st := &fakeStore{events: []domain.OutboxEvent{{ID: 7, SubmissionID: "sub-1", ClaimToken: "api-1", PublishAttempts: 1}}}
	pub := &fakePublisher{calls: &st.calls, err: errors.New("oops")}
	metrics := &fakeMetrics{}
	relay := outbox.New(st, pub, outbox.Config{
		RelayID:       "api-1",
		BatchSize:     10,
		ClaimDuration: 30 * time.Second,
		Metrics:       metrics,
	})
	_ = relay.PublishBatch(context.Background(), now)
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.results) != 1 || metrics.results[0] != "error" {
		t.Fatalf("results = %v, want [error]", metrics.results)
	}
}

func TestRelayRecordsMetricOnClaimLost(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	st := &fakeStore{
		events:     []domain.OutboxEvent{{ID: 7, SubmissionID: "sub-1", ClaimToken: "api-1", PublishAttempts: 1}},
		noMark:     true,
	}
	pub := &fakePublisher{calls: &st.calls}
	metrics := &fakeMetrics{}
	relay := outbox.New(st, pub, outbox.Config{
		RelayID:       "api-1",
		BatchSize:     10,
		ClaimDuration: 30 * time.Second,
		Metrics:       metrics,
	})
	_ = relay.PublishBatch(context.Background(), now)
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	if len(metrics.results) != 1 || metrics.results[0] != "claim_lost" {
		t.Fatalf("results = %v, want [claim_lost]", metrics.results)
	}
}

type fakeStore struct {
	events []domain.OutboxEvent
	calls  []string
	next   time.Time
	noMark bool
}

func (s *fakeStore) ClaimOutbox(context.Context, string, time.Time, time.Duration, int) ([]domain.OutboxEvent, error) {
	s.calls = append(s.calls, "claim")
	return s.events, nil
}
func (s *fakeStore) MarkOutboxPublished(context.Context, int64, string, time.Time) (bool, error) {
	if s.noMark {
		s.calls = append(s.calls, "published")
		return false, nil
	}
	s.calls = append(s.calls, "published")
	return true, nil
}
func (s *fakeStore) MarkOutboxFailed(_ context.Context, _ int64, _ string, next time.Time, _ string) (bool, error) {
	s.calls = append(s.calls, "failed")
	s.next = next
	return true, nil
}

type fakePublisher struct {
	calls *[]string
	err   error
	jobs  []domain.Job
}

func (p *fakePublisher) Enqueue(_ context.Context, job domain.Job) error {
	*p.calls = append(*p.calls, "publish")
	p.jobs = append(p.jobs, job)
	return p.err
}

func TestRelayPublishesThenMarksOutbox(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	st := &fakeStore{events: []domain.OutboxEvent{{ID: 7, SubmissionID: "sub-1", ClaimToken: "api-1", PublishAttempts: 1}}}
	pub := &fakePublisher{calls: &st.calls}
	relay := outbox.New(st, pub, outbox.Config{RelayID: "api-1", BatchSize: 10, ClaimDuration: 30 * time.Second})
	if err := relay.PublishBatch(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if want := []string{"claim", "publish", "published"}; !reflect.DeepEqual(st.calls, want) {
		t.Fatalf("calls = %v, want %v", st.calls, want)
	}
	if len(pub.jobs) != 1 || pub.jobs[0].SubmissionID != "sub-1" || pub.jobs[0].OutboxID != 7 {
		t.Fatalf("jobs = %+v", pub.jobs)
	}
}

func TestRelayRecordsFailureWithBoundedBackoff(t *testing.T) {
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	st := &fakeStore{events: []domain.OutboxEvent{{ID: 7, SubmissionID: "sub-1", ClaimToken: "api-1", PublishAttempts: 3}}}
	pub := &fakePublisher{calls: &st.calls, err: errors.New("redis unavailable")}
	relay := outbox.New(st, pub, outbox.Config{RelayID: "api-1", BatchSize: 10, ClaimDuration: 30 * time.Second})
	if err := relay.PublishBatch(context.Background(), now); err == nil {
		t.Fatal("PublishBatch should report publication failure")
	}
	if want := []string{"claim", "publish", "failed"}; !reflect.DeepEqual(st.calls, want) {
		t.Fatalf("calls = %v, want %v", st.calls, want)
	}
	if want := now.Add(time.Second); !st.next.Equal(want) {
		t.Fatalf("next attempt = %v, want %v", st.next, want)
	}
}
