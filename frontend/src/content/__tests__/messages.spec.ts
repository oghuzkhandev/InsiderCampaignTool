import { describe, expect, it } from 'vitest'
import { ApiError } from '@/api/campaigns'
import { contentErrorTitle, contentReasonMessage } from '../messages'

const codes = [
  'CONTENT_INVALID',
  'LLM_TIMEOUT',
  'LLM_RATE_LIMITED',
  'STREAM_INTERRUPTED',
  'CAMPAIGN_NOT_DRAFT',
  'CAMPAIGN_NOT_FOUND',
  'STALE_CONTENT_VERSION',
  'NO_VALIDATED_CONTENT',
  'CAMPAIGN_ALREADY_APPROVED',
  'RENDER_LIMIT_EXCEEDED',
  'NO_ELIGIBLE_RECIPIENTS',
  'UNFILTERED_SEGMENT',
  'REQUEST_TOO_LARGE',
  'INVALID_REQUEST',
  'NETWORK',
  'UNEXPECTED_RESPONSE',
  'INTERNAL',
]

describe('contentErrorTitle', () => {
  it.each(codes)('%s için Türkçe bir başlık verir (ham kod sızmaz)', (code) => {
    const title = contentErrorTitle(new ApiError(0, code, ''))
    expect(title.length).toBeGreaterThan(10)
    expect(title).not.toContain(code)
    expect(title).toMatch(/[.]$/)
  })

  it('bilinmeyen kodda sunucu mesajına, o da yoksa genel metne döner', () => {
    expect(contentErrorTitle(new ApiError(500, 'YENI_KOD', 'Sunucu mesajı'))).toBe('Sunucu mesajı')
    expect(contentErrorTitle(new ApiError(500, 'YENI_KOD', ''))).toBe('İstek tamamlanamadı.')
  })
})

describe('contentReasonMessage', () => {
  it('uzunluk aşımında alanı ve sınırı söyler', () => {
    expect(contentReasonMessage({ code: 'TOO_LONG', field: 'sms_text' })).toContain('160')
    expect(contentReasonMessage({ code: 'TOO_LONG', field: 'email_subject' })).toContain('80')
    expect(contentReasonMessage({ code: 'TOO_LONG', field: 'sms_text' })).toContain('SMS metni')
  })

  it('onaydaki aşımda alıcı sayısını ve örnek adı gösterir', () => {
    const msg = contentReasonMessage({
      code: 'TOO_LONG',
      field: 'sms_text',
      count: 12,
      text: 'Zeynep',
    })
    expect(msg).toContain('12 alıcı')
    expect(msg).toContain('"Zeynep"')
    expect(msg).toContain('160')
  })

  it('bilinmeyen değişkeni adıyla ve izinli değişkenle birlikte açıklar', () => {
    const msg = contentReasonMessage({
      code: 'UNKNOWN_VARIABLE',
      field: 'email_body',
      text: 'city',
    })
    expect(msg).toContain('E-posta gövdesi')
    expect(msg).toContain('{{city}}')
    expect(msg).toContain('{{first_name}}')
  })

  it.each([
    ['MALFORMED_VARIABLE', 'sms_text'],
    ['EMPTY_FIELD', 'email_subject'],
    ['MISSING_FIELD', 'email_body'],
    ['INVALID_CHARACTER', 'email_subject'],
    ['INVALID_CHARACTER', 'sms_text'],
    ['MALFORMED_OUTPUT', undefined],
  ])('%s (%s) Türkçe ve kodsuz', (code, field) => {
    const msg = contentReasonMessage({ code, field })
    expect(msg.length).toBeGreaterThan(8)
    expect(msg).not.toContain(code)
  })

  it('konudaki satır sonunu açıkça söyler; bilinmeyen kodda genel metin verir', () => {
    expect(contentReasonMessage({ code: 'INVALID_CHARACTER', field: 'email_subject' })).toContain(
      'satır sonu',
    )
    expect(contentReasonMessage({ code: 'YENI' })).toBe(
      'Bilinmeyen bir sebep nedeniyle reddedildi.',
    )
  })

  it('kullanıcı/model metnini olduğu gibi (HTML yorumlanmadan) metin olarak döndürür', () => {
    const msg = contentReasonMessage({
      code: 'UNKNOWN_VARIABLE',
      field: 'sms_text',
      text: '<img onerror=x>',
    })
    expect(msg).toContain('<img onerror=x>') // Vue metin olarak basar (v-html yok)
  })
})
