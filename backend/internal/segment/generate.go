package segment

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
)

var (
	// ErrEmptyBrief, boş veya yalnızca boşluktan oluşan brief için döner.
	ErrEmptyBrief = errors.New("segment: brief boş")
	// ErrBriefTooLong, brief MaxBriefRunes sınırını aşınca döner.
	ErrBriefTooLong = errors.New("segment: brief çok uzun")
)

// InvalidError, modelin çıktısı doğrulamadan geçemediğinde döner. API bunu
// 422 SEGMENT_INVALID ve sebep kodlarıyla iletir.
type InvalidError struct {
	Issues []Issue
}

func (e *InvalidError) Error() string { return "segment: geçersiz segment" }

const (
	defaultTimeout    = 30 * time.Second
	maxPreviousRunes  = 4000
	repairProblemsMax = 20
)

// Generator, brief'ten doğrulanmış segment üretir.
type Generator struct {
	LLM llm.Client
	// Timeout, her model çağrısı için süredir. Sıfırsa 30 saniye kullanılır.
	Timeout time.Duration
}

// Generate, brief'i doğrular, modeli çağırır ve çıktıyı doğrular. Yalnızca söz
// dizimi hatasında bir düzeltme denemesi yapılır. Zaman aşımı ve 429 hataları
// llm.ErrTimeout ve llm.ErrRateLimited olarak döner, düzeltme yapılmaz.
func (g *Generator) Generate(ctx context.Context, brief string) (Segment, error) {
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return Segment{}, ErrEmptyBrief
	}
	if utf8.RuneCountInString(brief) > MaxBriefRunes {
		return Segment{}, ErrBriefTooLong
	}

	out, err := g.call(ctx, llm.SegmentRequest{Brief: brief})
	if err != nil {
		return Segment{}, err
	}
	seg, issues := Validate([]byte(out))
	if len(issues) == 0 {
		return seg, nil
	}
	if !Repairable(issues) {
		return Segment{}, &InvalidError{Issues: issues}
	}

	problems := make([]string, 0, len(issues))
	for _, is := range issues {
		if len(problems) == repairProblemsMax {
			break
		}
		problems = append(problems, is.String())
	}
	out, err = g.call(ctx, llm.SegmentRequest{
		Brief:          brief,
		PreviousOutput: clip(out, maxPreviousRunes),
		Problems:       problems,
	})
	if err != nil {
		return Segment{}, err
	}
	seg, issues = Validate([]byte(out))
	if len(issues) > 0 {
		return Segment{}, &InvalidError{Issues: issues}
	}
	return seg, nil
}

func (g *Generator) call(ctx context.Context, req llm.SegmentRequest) (string, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := g.LLM.GenerateSegment(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", llm.ErrTimeout
		}
		return "", err
	}
	return out, nil
}
