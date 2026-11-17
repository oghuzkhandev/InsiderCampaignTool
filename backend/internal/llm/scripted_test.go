package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestScriptedReturnsStepsInOrderAndRecordsCalls(t *testing.T) {
	s := NewScripted(Step{Output: "a"}, Step{Err: ErrRateLimited})

	out, err := s.GenerateSegment(context.Background(), SegmentRequest{Brief: "b1"})
	if err != nil || out != "a" {
		t.Fatalf("ilk çağrı = %q, %v", out, err)
	}
	if _, err := s.GenerateSegment(context.Background(), SegmentRequest{Brief: "b2"}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("ikinci çağrı = %v", err)
	}
	if _, err := s.GenerateSegment(context.Background(), SegmentRequest{Brief: "b3"}); err == nil {
		t.Fatal("adım kalmadığında hata beklenir")
	}

	calls := s.Calls()
	if len(calls) != 3 || calls[0].Brief != "b1" || calls[2].Brief != "b3" {
		t.Fatalf("çağrılar = %#v", calls)
	}
}

func TestScriptedDelayHonorsContext(t *testing.T) {
	s := NewScripted(Step{Output: "geç", Delay: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := s.GenerateSegment(ctx, SegmentRequest{Brief: "x"})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("ErrTimeout bekleniyordu: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("bağlam iptali beklemeyi bitirmeli")
	}
}
