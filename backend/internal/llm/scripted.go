package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	// Chunks, yalnızca GenerateContent'te kullanılır: parçalar sırayla onChunk
	// ile iletilir. Err de doluysa parçalardan sonra hata döner (kesilen akışı
	// taklit eder, ör. ErrStreamInterrupted). Output boşsa tamamlanan metin
	// parçaların birleşimidir.
	Chunks []string
}

// Scripted, testler için önceden yazılmış yanıtları sırayla döndüren bir
// istemcidir. Segment ve içerik çağrıları aynı adım kuyruğunu paylaşır; testler
// çağrı sırasını açıkça yazar. Gelen istekleri kaydeder.
type Scripted struct {
	mu           sync.Mutex
	steps        []Step
	calls        []SegmentRequest
	contentCalls []ContentRequest
}

var (
	_ Client        = (*Scripted)(nil)
	_ ContentClient = (*Scripted)(nil)
)

// NewScripted, verilen adımlarla bir Scripted oluşturur.
func NewScripted(steps ...Step) *Scripted {
	return &Scripted{steps: steps}
}

// GenerateSegment sıradaki adımı uygular. Adım kalmadıysa hata döner; böylece
// beklenenden fazla çağrı testte görünür olur.
func (s *Scripted) GenerateSegment(ctx context.Context, req SegmentRequest) (string, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req)
	step, err := s.next(len(s.calls))
	s.mu.Unlock()
	if err != nil {
		return "", err
	}

	if err := wait(ctx, step.Delay); err != nil {
		return "", err
	}
	if step.Err != nil {
		return "", step.Err
	}
	return step.text(), nil
}

// GenerateContent sıradaki adımı uygular. Parçalar varsa onChunk ile iletilir;
// bağlam iptal olursa akış durur. Hata dönerse metin boştur.
func (s *Scripted) GenerateContent(ctx context.Context, req ContentRequest, onChunk func(string)) (string, error) {
	s.mu.Lock()
	s.contentCalls = append(s.contentCalls, req)
	step, err := s.next(len(s.contentCalls))
	s.mu.Unlock()
	if err != nil {
		return "", err
	}

	if err := wait(ctx, step.Delay); err != nil {
		return "", err
	}

	chunks := step.Chunks
	if len(chunks) == 0 && step.Err == nil && step.Output != "" {
		chunks = []string{step.Output}
	}
	for _, c := range chunks {
		if ctx.Err() != nil {
			return "", ctxError(ctx)
		}
		if onChunk != nil {
			onChunk(c)
		}
	}
	if step.Err != nil {
		return "", step.Err
	}
	return step.text(), nil
}

// next, kuyruğun başındaki adımı çıkarır. s.mu kilitliyken çağrılır.
func (s *Scripted) next(callNo int) (Step, error) {
	if len(s.steps) == 0 {
		return Step{}, fmt.Errorf("llm: scripted adım kalmadı (çağrı %d)", callNo)
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	return step, nil
}

func (st Step) text() string {
	if st.Output != "" || len(st.Chunks) == 0 {
		return st.Output
	}
	return strings.Join(st.Chunks, "")
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctxError(ctx)
	}
}

// Calls, şimdiye kadar gelen segment isteklerinin kopyasını döndürür.
func (s *Scripted) Calls() []SegmentRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SegmentRequest(nil), s.calls...)
}

// ContentCalls, şimdiye kadar gelen içerik isteklerinin kopyasını döndürür.
func (s *Scripted) ContentCalls() []ContentRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ContentRequest(nil), s.contentCalls...)
}

func ctxError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrTimeout
	}
	return ctx.Err()
}
