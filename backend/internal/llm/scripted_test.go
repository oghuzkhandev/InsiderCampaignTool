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

func TestScriptedContentStreamsChunksAndReturnsJoinedText(t *testing.T) {
	s := NewScripted(Step{Chunks: []string{`{"a":`, `"b"`, `}`}})
	var got []string
	out, err := s.GenerateContent(context.Background(), ContentRequest{Brief: "b1"}, func(c string) { got = append(got, c) })
	if err != nil || out != `{"a":"b"}` {
		t.Fatalf("çıktı = %q, %v", out, err)
	}
	if len(got) != 3 || got[1] != `"b"` {
		t.Fatalf("parçalar = %#v", got)
	}
	if calls := s.ContentCalls(); len(calls) != 1 || calls[0].Brief != "b1" {
		t.Fatalf("içerik çağrıları = %#v", calls)
	}
	if len(s.Calls()) != 0 {
		t.Fatal("segment çağrısı kaydedilmemeli")
	}
}

func TestScriptedContentWithoutChunksEmitsOutputOnce(t *testing.T) {
	s := NewScripted(Step{Output: `{"x":1}`})
	var got []string
	out, err := s.GenerateContent(context.Background(), ContentRequest{}, func(c string) { got = append(got, c) })
	if err != nil || out != `{"x":1}` || len(got) != 1 {
		t.Fatalf("çıktı = %q, %v, parçalar = %v", out, err, got)
	}
}

func TestScriptedContentInterruptedStreamDeliversPartialThenErrorWithEmptyText(t *testing.T) {
	s := NewScripted(Step{Chunks: []string{`{"sms_text":"Mer`, `haba`}, Err: ErrStreamInterrupted})
	var got []string
	out, err := s.GenerateContent(context.Background(), ContentRequest{}, func(c string) { got = append(got, c) })
	if !errors.Is(err, ErrStreamInterrupted) {
		t.Fatalf("ErrStreamInterrupted bekleniyordu: %v", err)
	}
	if out != "" {
		t.Fatalf("hata durumunda metin boş olmalı: %q", out)
	}
	if len(got) != 2 {
		t.Fatalf("kesilmeden önceki parçalar iletilmeli: %#v", got)
	}
}

func TestScriptedContentErrorsWithoutChunks(t *testing.T) {
	for _, want := range []error{ErrTimeout, ErrRateLimited} {
		s := NewScripted(Step{Err: want})
		out, err := s.GenerateContent(context.Background(), ContentRequest{}, func(string) { t.Fatal("parça iletilmemeli") })
		if !errors.Is(err, want) || out != "" {
			t.Fatalf("%v bekleniyordu: %q, %v", want, out, err)
		}
	}
}

func TestScriptedContentNilCallbackAndExhaustedSteps(t *testing.T) {
	s := NewScripted(Step{Output: "a"})
	if out, err := s.GenerateContent(context.Background(), ContentRequest{}, nil); err != nil || out != "a" {
		t.Fatalf("nil onChunk ile çalışmalı: %q, %v", out, err)
	}
	if _, err := s.GenerateContent(context.Background(), ContentRequest{}, nil); err == nil {
		t.Fatal("adım kalmadığında hata beklenir")
	}
}

func TestScriptedContentDelayHonorsContext(t *testing.T) {
	s := NewScripted(Step{Output: "geç", Delay: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := s.GenerateContent(ctx, ContentRequest{}, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ErrTimeout bekleniyordu: %v", err)
	}
}

func TestScriptedContentStopsStreamingWhenContextCanceled(t *testing.T) {
	s := NewScripted(Step{Chunks: []string{"a", "b", "c"}})
	ctx, cancel := context.WithCancel(context.Background())
	var got []string
	out, err := s.GenerateContent(ctx, ContentRequest{}, func(c string) {
		got = append(got, c)
		cancel()
	})
	if !errors.Is(err, context.Canceled) || out != "" || len(got) != 1 {
		t.Fatalf("iptalde akış durmalı: %q, %v, %v", out, err, got)
	}
}
