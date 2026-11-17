// Package llm, segment üretimi için model istemcisi arayüzünü ve test ile demo
// için kullanılan MockLLM'i içerir. Testler gerçek API anahtarı gerektirmez.
package llm

import (
	"context"
	"errors"
)

var (
	// ErrTimeout, model çağrısının zaman aşımına uğradığını gösterir.
	ErrTimeout = errors.New("llm: zaman aşımı")
	// ErrRateLimited, modelin 429 döndürdüğünü gösterir.
	ErrRateLimited = errors.New("llm: istek sınırı aşıldı (429)")
)

// SegmentRequest, bir segment üretim (veya düzeltme) isteğidir.
type SegmentRequest struct {
	// Brief, kullanıcının yazdığı güvenilmeyen metindir.
	Brief string
	// PreviousOutput ve Problems yalnızca düzeltme denemesinde doludur.
	PreviousOutput string
	Problems       []string
}

// Client, bir LLM sağlayıcısıdır. GenerateSegment yalnızca ham JSON metni
// döndürür; doğrulama çağıranın işidir.
type Client interface {
	GenerateSegment(ctx context.Context, req SegmentRequest) (string, error)
}
