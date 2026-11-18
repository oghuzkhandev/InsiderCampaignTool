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

// ErrStreamInterrupted, içerik akışının tamamlanmadan kesildiğini gösterir.
var ErrStreamInterrupted = errors.New("llm: akış yarıda kesildi")

// ContentRequest, bir içerik üretim (veya düzeltme) isteğidir.
type ContentRequest struct {
	// Brief, kullanıcının yazdığı güvenilmeyen metindir.
	Brief string
	// PreviousOutput ve Problems yalnızca düzeltme denemesinde doludur.
	PreviousOutput string
	Problems       []string
}

// ContentClient, SMS ve e-posta içeriğini akışla üreten bir LLM sağlayıcısıdır.
// Client'tan ayrıdır: segment üretimi akış gerektirmez ve Client'ı kullanan kod
// değişmez (ADR-022).
//
// GenerateContent, modelin çıktısını ham metin parçaları olarak onChunk ile
// iletir (onChunk nil olabilir) ve tamamlanınca bütün JSON metnini döndürür.
// Doğrulama çağıranın işidir. Hata durumunda dönen metin boştur; önceden
// iletilmiş parçalar yalnızca geçici önizlemedir ve kaydedilmemelidir. Akış
// yarıda kesilirse ErrStreamInterrupted, zaman aşımında ErrTimeout, 429'da
// ErrRateLimited döner.
type ContentClient interface {
	GenerateContent(ctx context.Context, req ContentRequest, onChunk func(chunk string)) (string, error)
}

// Provider, hem segment hem içerik üretebilen sağlayıcıdır (Mock, Scripted ve
// ileride gerçek Claude adaptörü).
type Provider interface {
	Client
	ContentClient
}
