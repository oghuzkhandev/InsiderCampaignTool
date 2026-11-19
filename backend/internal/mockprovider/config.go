// Package mockprovider, SMS ve e-posta gönderimini taklit eden sahte sağlayıcıdır
// (SND-001, ARCHITECTURE §11, ADR-025). Gerçek hiçbir kişiye mesaj gitmez.
//
// Retry, idempotency ve hız sınırı mantığını gerçekten sınayabilmek için zaman
// zaman gecikme, 429 ve 500 üretir; ilk kabul yanıtını veya kabul edilen
// isteğin bütün yanıtlarını düşürebilir. İlk kabul PostgreSQL'e atomik
// kaydedilir, bu yüzden sağlayıcı yeniden başlasa da korunur.
package mockprovider

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Mode, başarılı bir kabulün yanıtının ne olacağını belirler.
type Mode string

const (
	// ModeNormal: kabul edilen her istek yanıtını alır.
	ModeNormal Mode = ""
	// ModeAcceptThenDrop: bir anahtarın yalnızca İLK kabul yanıtı düşürülür
	// (kayıt commit edildikten sonra). Aynı anahtarla gelen tekrar isteği
	// kayıtlı sonucu alır.
	ModeAcceptThenDrop Mode = "accept_then_drop"
	// ModeDropAllResponses: kabul kaydedilir ama kabul edilen isteğin HER
	// yanıtı düşürülür. Deneme sınırında worker mesajı `exhausted` ile kapatır.
	ModeDropAllResponses Mode = "drop_all_responses"
)

// Config, sağlayıcının hata ve gecikme davranışıdır. Sıfır değer: gecikmesiz ve
// hatasız.
type Config struct {
	// Latency, her isteğe eklenen sabit gecikmedir.
	Latency time.Duration
	// RateLimitRate, isteğin 429 ile reddedilme olasılığıdır (0..1).
	RateLimitRate float64
	// ServerErrorRate, isteğin 500 ile reddedilme olasılığıdır (0..1).
	ServerErrorRate float64
	// RetryAfter, 429 yanıtındaki Retry-After değeridir (saniyeye yukarı
	// yuvarlanır).
	RetryAfter time.Duration
	// Mode, kabul edilen isteklerin yanıt davranışıdır.
	Mode Mode
}

// Ortam değişkenleri.
const (
	EnvLatency       = "MOCK_PROVIDER_LATENCY"
	EnvRateLimitRate = "MOCK_PROVIDER_429_RATE"
	EnvErrorRate     = "MOCK_PROVIDER_500_RATE"
	EnvRetryAfter    = "MOCK_PROVIDER_RETRY_AFTER"
	EnvMode          = "MOCK_PROVIDER_MODE"
)

// Validate, yapılandırmanın geçerli olduğunu doğrular.
func (c Config) Validate() error {
	if c.Latency < 0 {
		return fmt.Errorf("mockprovider: gecikme negatif olamaz: %s", c.Latency)
	}
	if c.RetryAfter < 0 {
		return fmt.Errorf("mockprovider: Retry-After negatif olamaz: %s", c.RetryAfter)
	}
	if c.RateLimitRate < 0 || c.RateLimitRate > 1 {
		return fmt.Errorf("mockprovider: 429 oranı 0..1 aralığında olmalı: %v", c.RateLimitRate)
	}
	if c.ServerErrorRate < 0 || c.ServerErrorRate > 1 {
		return fmt.Errorf("mockprovider: 500 oranı 0..1 aralığında olmalı: %v", c.ServerErrorRate)
	}
	switch c.Mode {
	case ModeNormal, ModeAcceptThenDrop, ModeDropAllResponses:
	default:
		return fmt.Errorf("mockprovider: bilinmeyen mod %q", c.Mode)
	}
	return nil
}

// ConfigFromEnv, yapılandırmayı ortam değişkenlerinden okur. Tanımsız değişken
// sıfır değerde kalır; geçersiz değer hata verir.
func ConfigFromEnv() (Config, error) {
	var c Config
	var err error
	if c.Latency, err = envDuration(EnvLatency); err != nil {
		return Config{}, err
	}
	if c.RetryAfter, err = envDuration(EnvRetryAfter); err != nil {
		return Config{}, err
	}
	if c.RateLimitRate, err = envFloat(EnvRateLimitRate); err != nil {
		return Config{}, err
	}
	if c.ServerErrorRate, err = envFloat(EnvErrorRate); err != nil {
		return Config{}, err
	}
	c.Mode = Mode(os.Getenv(EnvMode))
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func envDuration(name string) (time.Duration, error) {
	v := os.Getenv(name)
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("mockprovider: %s geçersiz süre %q: %w", name, v, err)
	}
	return d, nil
}

func envFloat(name string) (float64, error) {
	v := os.Getenv(name)
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("mockprovider: %s geçersiz sayı %q: %w", name, v, err)
	}
	return f, nil
}
