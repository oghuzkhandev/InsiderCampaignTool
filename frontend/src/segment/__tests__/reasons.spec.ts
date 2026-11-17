import { describe, expect, it } from 'vitest'
import { reasonMessage } from '../reasons'

describe('reasonMessage', () => {
  it('desteklenmeyen alanı bilinen etiketle ve desteklenen alanları sayarak açıklar', () => {
    const msg = reasonMessage({ code: 'UNSUPPORTED_FIELD', field: 'favorite_color' })
    expect(msg).toContain('"favorite_color" alanı desteklenmiyor')
    expect(msg).toContain('şehir')
    expect(msg).toContain('satın alma tarihi')
  })

  it('bilinen alanı Türkçe etiketle gösterir', () => {
    expect(reasonMessage({ code: 'UNSUPPORTED_OPERATOR', field: 'city', text: 'like' })).toBe(
      'Şehir için "like" koşulu desteklenmiyor.',
    )
    expect(reasonMessage({ code: 'INVALID_RANGE', field: 'last_cart_add_at' })).toBe(
      'Sepete ekleme tarihi için değer izin verilen aralıkta değil.',
    )
  })

  it('alan adı yoksa genel ifade kullanır', () => {
    expect(reasonMessage({ code: 'INVALID_VALUE_TYPE' })).toBe(
      'Bir alan için değer geçerli bir türde değil.',
    )
  })

  it('ifade edilemeyen kısmı gösterir', () => {
    expect(reasonMessage({ code: 'UNSUPPORTED_PART', text: 'ürün kategorisi' })).toBe(
      'Brief\'in şu kısmı ifade edilemedi: "ürün kategorisi".',
    )
  })

  it('VEYA ve iç içe yapıyı açıklar', () => {
    expect(reasonMessage({ code: 'UNSUPPORTED_STRUCTURE' })).toContain("yalnızca 've' ile")
  })

  it('filtresiz kuralı açıklar ve ne yapılacağını söyler', () => {
    const msg = reasonMessage({ code: 'EMPTY_FILTER' })
    expect(msg).toContain('hiçbir koşul çıkarılamadı')
    expect(msg).toContain('en az bir koşul belirtin')
  })

  it('bozuk model çıktısını kullanıcıya teknik ayrıntı vermeden açıklar', () => {
    const msg = reasonMessage({ code: 'MALFORMED_OUTPUT' })
    expect(msg).toContain('daha açık yazıp yeniden deneyin')
    expect(msg).not.toMatch(/json/i)
  })

  it('bilinmeyen kodda genel mesaj verir', () => {
    expect(reasonMessage({ code: 'SOMETHING_NEW' })).toBe(
      'Bilinmeyen bir sebep nedeniyle reddedildi.',
    )
  })

  it('model kaynaklı metni değiştirmeden döndürür (HTML kaçışı arayüzün işidir)', () => {
    const text = '<img src=x onerror=alert(1)>'
    expect(reasonMessage({ code: 'UNSUPPORTED_PART', text })).toContain(text)
  })
})
