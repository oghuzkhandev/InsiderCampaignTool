// Package httpapi, API'nin HTTP katmanıdır: yönlendirme, istek ve yanıt
// dönüşümü, durum kodları ve hata eşlemesi.
//
// Handler'lar yalnızca HTTP işini yapar. İş kuralları ve doğrulama
// `segment`, `llm` ve `campaign` gibi paketlerde, SQL ise `campaign`,
// `audience`, `seed` ve `database` paketlerinde kalır. Handler'lar bu
// paketlere küçük arayüzlerle bağlanır; böylece `httptest` ile sınanabilir.
//
// `cmd/api` yalnızca bağlama yapar (yapılandırma, veritabanı, LLM istemcisi,
// sunucu). Bağımlılık yönü: cmd/api → httpapi → segment, audience, campaign,
// llm. Ters yönde bağımlılık olmaz.
package httpapi
