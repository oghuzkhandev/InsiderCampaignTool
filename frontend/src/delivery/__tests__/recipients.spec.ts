import { describe, expect, it } from 'vitest'
import {
  CHANNEL_OPTIONS,
  MESSAGE_STATUS_OPTIONS,
  failureReasonLabel,
  formatSentAt,
  messageStatusLabel,
  recipientInitial,
} from '../recipients'

describe('messageStatusLabel', () => {
  it('her durumun Türkçe adı vardır', () => {
    expect(MESSAGE_STATUS_OPTIONS.map((o) => o.label)).toEqual([
      'Bekleyen',
      'Gönderiliyor',
      'Gönderildi',
      'Başarısız',
      'Atlandı',
    ])
  })

  it('denenmiş ve yeniden sırada bekleyen mesaj "Yeniden denenecek"tir', () => {
    expect(messageStatusLabel('queued', 0)).toBe('Bekleyen')
    expect(messageStatusLabel('queued', 2)).toBe('Yeniden denenecek')
    expect(messageStatusLabel('sent', 3)).toBe('Gönderildi')
  })

  it('kanal seçenekleri SMS ve E-posta', () => {
    expect(CHANNEL_OPTIONS).toEqual([
      { value: 'sms', label: 'SMS' },
      { value: 'email', label: 'E-posta' },
    ])
  })
})

describe('failureReasonLabel', () => {
  it('bilinen kodları açıklar, bilinmeyeni göstermez', () => {
    expect(failureReasonLabel('exhausted')).toBe('Deneme sınırı doldu')
    expect(failureReasonLabel('rejected')).toBe('Sağlayıcı reddetti')
    expect(failureReasonLabel('<script>')).toBeNull()
    expect(failureReasonLabel(null)).toBeNull()
  })
})

describe('recipientInitial', () => {
  it('adın ilk harfini Türkçe büyük harfle verir', () => {
    expect(recipientInitial('ışık', 'x@y.z')).toBe('I')
    expect(recipientInitial('izel', 'x@y.z')).toBe('İ')
  })

  it('ad yoksa veya boşsa adresin ilk harfini, o da yoksa "?" verir', () => {
    expect(recipientInitial(null, 'mehmet@y.z')).toBe('M')
    expect(recipientInitial('   ', '+905551112233')).toBe('+')
    expect(recipientInitial(null, '')).toBe('?')
  })
})

describe('formatSentAt', () => {
  it('geçerli zamanı gün, ay ve saatle yazar', () => {
    expect(formatSentAt('2026-11-20T06:54:05Z')).toMatch(/^20 Kas \d{2}:\d{2}$/)
  })

  it('yok veya geçersiz zamanda null verir', () => {
    expect(formatSentAt(null)).toBeNull()
    expect(formatSentAt('dün')).toBeNull()
  })
})
