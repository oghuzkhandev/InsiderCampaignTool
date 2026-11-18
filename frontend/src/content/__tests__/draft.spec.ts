import { describe, expect, it } from 'vitest'
import {
  FALLBACK_NAME,
  applyDelta,
  emptyFields,
  estimatedLength,
  hasText,
  runeLength,
  sameFields,
} from '../draft'

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
