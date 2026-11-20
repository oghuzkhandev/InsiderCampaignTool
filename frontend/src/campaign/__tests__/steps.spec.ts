import { describe, expect, it } from 'vitest'
import { STEPS, stepStatuses, stepStatusText } from '../steps'

const states = (status: string | null, contentReady = false) =>
  STEPS.map((s) => stepStatuses({ status, contentReady })[s.key])

describe('adımlar', () => {
  it('dört adım ve sırası: kitle, içerik, onay, gönderim', () => {
    expect(STEPS.map((s) => s.label)).toEqual(['Kitle', 'İçerik', 'Onay', 'Gönderim'])
  })

  it('kampanya yokken kitle adımı sürer', () => {
    expect(states(null)).toEqual(['current', 'upcoming', 'upcoming', 'upcoming'])
  })

  it('bilinmeyen durumda kitle adımı sürer (varsayılan)', () => {
    expect(states('bilinmiyor')).toEqual(['current', 'upcoming', 'upcoming', 'upcoming'])
  })

  it('taslakta içerik hazır değilse içerik adımı sürer', () => {
    expect(states('draft', false)).toEqual(['done', 'current', 'upcoming', 'upcoming'])
  })

  it('taslakta kaydedilmiş içerik varsa onay adımı sürer', () => {
    expect(states('draft', true)).toEqual(['done', 'done', 'current', 'upcoming'])
  })

  it('onaylanmış ve gönderilen kampanyada gönderim adımı sürer', () => {
    expect(states('approved')).toEqual(['done', 'done', 'done', 'current'])
    expect(states('sending')).toEqual(['done', 'done', 'done', 'current'])
  })

  it('onaylı kampanyada içeriğin hazır olup olmadığı sonucu değiştirmez', () => {
    expect(states('approved', false)).toEqual(states('approved', true))
  })

  it('tamamlanmış kampanyada dört adım da bitmiştir', () => {
    expect(states('completed')).toEqual(['done', 'done', 'done', 'done'])
  })

  it('her durumda en çok bir adım "şu anki" olur', () => {
    for (const status of [null, 'draft', 'approved', 'sending', 'completed']) {
      for (const ready of [false, true]) {
        expect(states(status, ready).filter((s) => s === 'current').length).toBeLessThanOrEqual(1)
      }
    }
  })

  it('ekran okuyucu metinleri', () => {
    expect(stepStatusText('done')).toBe('tamamlandı')
    expect(stepStatusText('current')).toBe('şu anki adım')
    expect(stepStatusText('upcoming')).toBe('bekliyor')
  })
})
