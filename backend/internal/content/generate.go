package content

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/oghuzkhandev/InsiderCampaignTool/backend/internal/llm"
)

// ErrEmptyBrief, boş veya yalnızca boşluktan oluşan brief için döner.
var ErrEmptyBrief = errors.New("content: brief boş")

// InvalidError, otomatik düzeltmeden sonra da içeriğin doğrulanamadığını
// gösterir. Yeni sürüm kaydedilmez ve onay kapalı kalır. Partial, çıktıdan
// çözümlenebilen doğrulanmamış metinlerdir; yalnızca insan düzenlemesine
// başlangıç olarak sunulur. Hiç metin çözümlenemediyse boştur.
type InvalidError struct {
	Issues  []Issue
	Partial Content
}

func (e *InvalidError) Error() string { return "content: geçersiz içerik" }

const (
	// defaultTimeout, her model çağrısı için süredir. Akış segment çağrısından
	// uzun sürebilir.
	defaultTimeout    = 60 * time.Second
	maxPreviousRunes  = 8000
	repairProblemsMax = 20
)

// Generator, brief'ten doğrulanmış içerik üretir.
type Generator struct {
	LLM llm.ContentClient
	// Timeout, her model çağrısı için süredir. Sıfırsa 60 saniye kullanılır.
	Timeout time.Duration
}

// Generate, modeli çağırır, çıktıyı akış sırasında onDelta ile geçici önizleme
// olarak iletir (nil olabilir) ve tamamlanınca doğrular.
//
// Geçerliyse içerik döner; kaydetmek çağıranın işidir. Tamamlanmış ama geçersiz
// çıktıda doğrulama sorunlarıyla bir kez düzeltme denenir. Düzeltme sırasında
// onDelta çağrılmaz (çıktı arka planda biriktirilir); çağıran doğrulanan
// içerikle önizlemeyi tümüyle değiştirir. Düzeltme de başarısızsa *InvalidError
// döner.
//
// Zaman aşımı, 429 ve kesilen akış llm.ErrTimeout, llm.ErrRateLimited ve
// llm.ErrStreamInterrupted olarak döner; düzeltme denenmez. Düzeltme çağrısı
// kendisi hata verirse o hata döner.
func (g *Generator) Generate(ctx context.Context, brief string, opts Options, onDelta func(Delta)) (Content, error) {
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return Content{}, ErrEmptyBrief
	}

	var parser deltaParser
	out, err := g.call(ctx, llm.ContentRequest{Brief: brief}, func(chunk string) {
		if onDelta == nil {
			return
		}
		for _, d := range parser.Feed(chunk) {
			onDelta(d)
		}
	})
	if err != nil {
		return Content{}, err
	}
	c, issues := Validate([]byte(out), opts)
	if len(issues) == 0 {
		return c, nil
	}

	problems := make([]string, 0, len(issues))
	for _, is := range issues {
		if len(problems) == repairProblemsMax {
			break
		}
		problems = append(problems, is.String())
	}
	repaired, err := g.call(ctx, llm.ContentRequest{
		Brief:          brief,
		PreviousOutput: clipRunes(out, maxPreviousRunes),
		Problems:       problems,
	}, nil)
	if err != nil {
		return Content{}, err
	}
	c, issues = Validate([]byte(repaired), opts)
	if len(issues) == 0 {
		return c, nil
	}

	partial := Partial(repaired)
	if partial == (Content{}) {
		partial = Partial(out)
	}
	return Content{}, &InvalidError{Issues: issues, Partial: partial}
}

func (g *Generator) call(ctx context.Context, req llm.ContentRequest, onChunk func(string)) (string, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := g.LLM.GenerateContent(ctx, req, onChunk)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", llm.ErrTimeout
		}
		return "", err
	}
	return out, nil
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
