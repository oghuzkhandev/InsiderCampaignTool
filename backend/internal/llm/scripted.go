package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Step, Scripted istemcisinin tek bir çağrıya vereceği yanıttır.
type Step struct {
	Output string
	Err    error
	// Delay, yanıttan önce beklenecek süredir. Bağlam iptal olursa beklemeden
	// çıkılır.
	Delay time.Duration
}

// Scripted, testler için önceden yazılmış yanıtları sırayla döndüren bir
// istemcidir. Gelen istekleri kaydeder.
type Scripted struct {
	mu    sync.Mutex
	steps []Step
	calls []SegmentRequest
}

// NewScripted, verilen adımlarla bir Scripted oluşturur.
func NewScripted(steps ...Step) *Scripted {
	return &Scripted{steps: steps}
}

// GenerateSegment sıradaki adımı uygular. Adım kalmadıysa hata döner; böylece
// beklenenden fazla çağrı testte görünür olur.
func (s *Scripted) GenerateSegment(ctx context.Context, req SegmentRequest) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req)
	if len(s.steps) == 0 {
		s.mu.Unlock()
		return "", fmt.Errorf("llm: scripted adım kalmadı (çağrı %d)", len(s.calls))
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	s.mu.Unlock()

	if step.Delay > 0 {
		select {
		case <-time.After(step.Delay):
		case <-ctx.Done():
			return "", ctxError(ctx)
		}
	}
	if step.Err != nil {
		return "", step.Err
	}
	return step.Output, nil
}

// Calls, şimdiye kadar gelen isteklerin kopyasını döndürür.
func (s *Scripted) Calls() []SegmentRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SegmentRequest(nil), s.calls...)
}

func ctxError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrTimeout
	}
	return ctx.Err()
}
