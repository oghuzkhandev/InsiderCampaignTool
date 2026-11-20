import { describe, expect, it } from 'vitest'
import {
  AUDIENCE_LIST_NOTE,
  MISSING,
  channelStatusHint,
  channelStatusLabel,
  pageRange,
  totalPages,
} from '../members'
import type { ChannelStatus } from '@/api/audience'

const STATUSES: ChannelStatus[] = ['eligible', 'no_consent', 'no_address']

describe('totalPages', () => {
  it.each([
    [0, 10, 1], // hiç kişi yok: yine de en az bir sayfa
    [1, 10, 1],
    [10, 10, 1],
    [11, 10, 2],
    [191, 10, 20],
    [200, 10, 20],
    [201, 10, 21],
  ])('%i kişi, sayfada %i: %i sayfa', (total, limit, want) => {
    expect(totalPages(total, limit)).toBe(want)
  })

  it('geçersiz sayfa boyutunda tek sayfa varsayar (sıfıra bölmez)', () => {
    expect(totalPages(50, 0)).toBe(1)
  })
})

describe('pageRange', () => {
  it('sayfadaki kişilerin sıra aralığını verir', () => {
    expect(pageRange(1, 10, 10)).toEqual({ from: 1, to: 10 })
    expect(pageRange(3, 10, 10)).toEqual({ from: 21, to: 30 })
  })

  it('kısa son sayfada gerçek kişi sayısını kullanır', () => {
    expect(pageRange(3, 10, 5)).toEqual({ from: 21, to: 25 })
  })

  it('boş sayfa için null döner', () => {
    expect(pageRange(9, 10, 0)).toBeNull()
  })
})

describe('kanal uygunluğu metinleri', () => {
  it('her durumun kısa, ayrı bir etiketi vardır', () => {
    expect(STATUSES.map(channelStatusLabel)).toEqual(['Uygun', 'İzin yok', 'Adres yok'])
  })

  it('her kanal ve durum için farklı, dolu bir ipucu vardır', () => {
    const hints = (['sms', 'email'] as const).flatMap((channel) =>
      STATUSES.map((status) => channelStatusHint(channel, status)),
    )
    expect(hints.every((h) => h.length > 10)).toBe(true)
    expect(new Set(hints).size).toBe(hints.length)
  })

  it('izin yoksa o kanaldan mesaj gitmediğini söyler', () => {
    expect(channelStatusHint('sms', 'no_consent')).toContain('SMS gönderilmez')
    expect(channelStatusHint('email', 'no_consent')).toContain('e-posta gönderilmez')
  })

  it('adres eksikliğinde hangi bilginin eksik olduğunu söyler', () => {
    expect(channelStatusHint('sms', 'no_address')).toContain('telefon')
    expect(channelStatusHint('email', 'no_address')).toContain('e-posta adresi')
  })
})

describe('eksik bilgi metinleri', () => {
  it('ad, e-posta ve telefon yokluğu açıkça yazılır', () => {
    expect(MISSING).toEqual({ name: 'Ad yok', email: 'E-posta yok', phone: 'Telefon yok' })
  })
})

describe('liste notu', () => {
  it('demo verisi olduğunu ve kesin alıcıların onayda belirlendiğini söyler', () => {
    expect(AUDIENCE_LIST_NOTE).toContain('demo verisidir')
    expect(AUDIENCE_LIST_NOTE).toContain('onaylanırken yeniden belirlenir')
  })

  it('izinsiz kişilerin listelendiğini ama gönderim yapılmadığını söyler', () => {
    expect(AUDIENCE_LIST_NOTE).toContain('İzni olmayan')
    expect(AUDIENCE_LIST_NOTE).toContain('mesaj gönderilmez')
  })
})
