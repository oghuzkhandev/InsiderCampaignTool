import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusBadge from '../StatusBadge.vue'

const badge = (status: string) => mount(StatusBadge, { props: { status } })

describe('StatusBadge', () => {
  it.each([
    ['draft', 'Taslak'],
    ['approved', 'Onaylandı, kuyrukta'],
    ['sending', 'Gönderiliyor'],
    ['completed', 'Tamamlandı'],
  ])('%s durumu "%s" metnini ve bir simgeyi gösterir', (status, label) => {
    const w = badge(status)

    expect(w.text()).toBe(label)
    expect(w.find('svg').exists()).toBe(true)
    expect(w.attributes('data-status')).toBe(status)
  })

  it('simge süstür: ekran okuyucudan gizlidir, anlam metinle verilir', () => {
    const w = badge('completed')
    expect(w.get('svg').attributes('aria-hidden')).toBe('true')
  })

  it('her durumun kendine ait bir tonu ve simgesi vardır (durum yalnızca renkle anlatılmaz)', () => {
    const statuses = ['draft', 'approved', 'sending', 'completed']
    const tones = statuses.map((s) => badge(s).classes().join(' '))
    expect(new Set(tones).size).toBe(statuses.length)
    const icons = statuses.map((s) => badge(s).get('svg').classes().join(' '))
    expect(new Set(icons).size).toBe(statuses.length)
  })

  it('bilinmeyen durumu olduğu gibi yazar ve nötr bir simge kullanır', () => {
    const w = badge('archived')
    expect(w.text()).toBe('archived')
    expect(w.find('svg').exists()).toBe(true)
  })
})
