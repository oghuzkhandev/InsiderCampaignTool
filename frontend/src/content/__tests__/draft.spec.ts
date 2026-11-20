import { describe, expect, it } from 'vitest'
import {
  FALLBACK_NAME,
  applyDelta,
  emptyFields,
  estimatedLength,
  hasText,
  renderPreview,
  runeLength,
  sameFields,
  SAMPLE_NAME,
} from '../draft'

describe('renderPreview', () => {
  it('{{first_name}} yerine örnek adı koyar (her geçtiği yerde)', () => {
    expect(renderPreview('Merhaba {{first_name}}!')).toBe(`Merhaba ${SAMPLE_NAME}!`)
    expect(renderPreview('{{first_name}}, {{first_name}}')).toBe(`${SAMPLE_NAME}, ${SAMPLE_NAME}`)
  })

  it('verilen adı kullanır; değişkensiz ve başka süslü parantezli metne dokunmaz', () => {
    expect(renderPreview('Selam {{first_name}}', 'Mehmet')).toBe('Selam Mehmet')
    expect(renderPreview('değişkensiz')).toBe('değişkensiz')
    expect(renderPreview('{{other}} {first_name}')).toBe('{{other}} {first_name}')
  })

  it('adın içindeki süslü parantezleri yeniden yorumlamaz (tek geçiş)', () => {
    expect(renderPreview('{{first_name}}', '{{first_name}}')).toBe('{{first_name}}')
  })

  it('örnek ad gerçek bir kişi adı gibi sabit bir değerdir', () => {
    expect(SAMPLE_NAME.length).toBeGreaterThan(0)
    expect(SAMPLE_NAME).not.toContain('{')
  })
})

describe('runeLength', () => {
  it('UTF-16 uzunluğu değil Unicode kod noktası sayar', () => {
    expect(runeLength('ğüşiöç İ')).toBe(8)
    expect(runeLength('😀😀')).toBe(2) // UTF-16'da 4
    expect(runeLength('')).toBe(0)
  })
})

describe('estimatedLength', () => {
  it('{{first_name}} yerine fallback uzunluğunu varsayar', () => {
    expect(estimatedLength('Merhaba {{first_name}}!')).toBe(
      'Merhaba !'.length + runeLength(FALLBACK_NAME),
    )
    expect(estimatedLength('{{first_name}}{{first_name}}')).toBe(2 * runeLength(FALLBACK_NAME))
    expect(estimatedLength('değişkensiz')).toBe(11)
  })
})

describe('applyDelta', () => {
  it('replace önceki metni siler, değilse ekler; diğer alanlara dokunmaz', () => {
    let f = emptyFields()
    f = applyDelta(f, { field: 'sms_text', text: 'Mer', replace: true })
    f = applyDelta(f, { field: 'sms_text', text: 'haba', replace: false })
    f = applyDelta(f, { field: 'email_subject', text: 'Konu', replace: true })
    expect(f).toEqual({ sms_text: 'Merhaba', email_subject: 'Konu', email_body: '' })

    // Aynı alan yeniden başlarsa (replace) önizleme ona uyar.
    f = applyDelta(f, { field: 'sms_text', text: 'Yeni', replace: true })
    expect(f.sms_text).toBe('Yeni')
  })

  it('girdiyi değiştirmez', () => {
    const f = emptyFields()
    applyDelta(f, { field: 'sms_text', text: 'x', replace: true })
    expect(f).toEqual(emptyFields())
  })
})

describe('sameFields ve hasText', () => {
  it('alan alan karşılaştırır', () => {
    const a = { sms_text: 'a', email_subject: 'b', email_body: 'c' }
    expect(sameFields(a, { ...a })).toBe(true)
    expect(sameFields(a, { ...a, email_body: 'd' })).toBe(false)
  })

  it('yalnızca boşluk metin sayılmaz', () => {
    expect(hasText(emptyFields())).toBe(false)
    expect(hasText({ sms_text: ' \n', email_subject: '', email_body: '' })).toBe(false)
    expect(hasText({ sms_text: '', email_subject: '', email_body: 'x' })).toBe(true)
  })
})
