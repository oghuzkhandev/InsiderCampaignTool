import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ListPagination from '../ListPagination.vue'

function mountPager(
  props: Partial<{ page: number; total: number; limit: number; loading: boolean }> = {},
) {
  return mount(ListPagination, {
    props: { page: 1, total: 191, limit: 10, loading: false, ...props },
  })
}

const pageButtons = (w: ReturnType<typeof mountPager>) =>
  w.findAll('[data-slot="pagination-item"]').map((b) => b.text())
const prev = (w: ReturnType<typeof mountPager>) => w.get('[data-slot="pagination-previous"]')
const next = (w: ReturnType<typeof mountPager>) => w.get('[data-slot="pagination-next"]')
const summary = (w: ReturnType<typeof mountPager>) => w.get('p').text().replace(/\s+/g, ' ')

describe('ListPagination', () => {
  it('aralığı, toplamı ve sayfayı özetler', () => {
    expect(summary(mountPager({ page: 2 }))).toBe('11–20 / 191 kayıt · Sayfa 2 / 20')
  })

  it('son sayfada gerçek kayıt sayısına göre biter', () => {
    expect(summary(mountPager({ page: 20 }))).toBe('191–191 / 191 kayıt · Sayfa 20 / 20')
  })

  it('"Sayfalama" adlı bir gezinti bölgesidir', () => {
    expect(mountPager().get('nav').attributes('aria-label')).toBe('Sayfalama')
  })

  it('başta ve sonda sayfa, ortada üç nokta gösterir; etkin sayfa işaretlidir', () => {
    const w = mountPager({ page: 10 })

    const labels = pageButtons(w)
    expect(labels[0]).toBe('1')
    expect(labels[labels.length - 1]).toBe('20')
    expect(labels).toContain('10')
    expect(w.findAll('[data-slot="pagination-ellipsis"]').length).toBeGreaterThanOrEqual(1)
    const current = w.get('[aria-current="page"]')
    expect(current.text()).toBe('10')
    expect(current.attributes('aria-label')).toBe('Sayfa 10')
  })

  it('az sayfada hepsini sayı olarak gösterir', () => {
    expect(pageButtons(mountPager({ total: 35, page: 1 }))).toEqual(['1', '2', '3', '4'])
  })

  it('bir sayfa numarasına tıklayınca o sayfayı bildirir', async () => {
    const w = mountPager({ total: 35, page: 1 })

    await w.get('[aria-label="Sayfa 3"]').trigger('click')

    expect(w.emitted('change')).toEqual([[3]])
  })

  it('Sonraki ve Önceki bir sayfa ilerletir ve geri alır', async () => {
    const w = mountPager({ page: 5 })

    await next(w).trigger('click')
    await prev(w).trigger('click')

    expect(w.emitted('change')).toEqual([[6], [4]])
  })

  it('ilk sayfada Önceki, son sayfada Sonraki kapalıdır', () => {
    expect(prev(mountPager({ page: 1 })).attributes('disabled')).toBeDefined()
    expect(next(mountPager({ page: 1 })).attributes('disabled')).toBeUndefined()
    expect(next(mountPager({ page: 20 })).attributes('disabled')).toBeDefined()
    expect(prev(mountPager({ page: 20 })).attributes('disabled')).toBeUndefined()
  })

  it('düğmelerin erişilebilir adı Türkçedir', () => {
    const w = mountPager({ page: 2 })

    expect(prev(w).attributes('aria-label')).toBe('Önceki sayfa')
    expect(next(w).attributes('aria-label')).toBe('Sonraki sayfa')
  })

  it('yüklenirken bütün düğmeler kapanır ve tıklama bildirim yapmaz', async () => {
    const w = mountPager({ page: 5, loading: true })

    for (const b of w.findAll('button')) expect(b.attributes('disabled')).toBeDefined()
    await next(w).trigger('click')

    expect(w.emitted('change')).toBeUndefined()
  })

  it('tek sayfa varsa yalnızca özet görünür', () => {
    const w = mountPager({ total: 7 })

    expect(summary(w)).toBe('1–7 / 7 kayıt · Sayfa 1 / 1')
    expect(w.findAll('button')).toHaveLength(0)
  })

  it('kayıt yoksa "Kayıt yok" der ve düğme göstermez', () => {
    const w = mountPager({ total: 0 })

    expect(summary(w)).toBe('Kayıt yok')
    expect(w.findAll('button')).toHaveLength(0)
  })

  it('sayfa aşıldıysa (kayıt kısalmış) aralık yazmaz, yalnızca sayfayı', () => {
    expect(summary(mountPager({ page: 9, total: 25 }))).toBe('Sayfa 9 / 3')
  })

  it('dokunma hedefleri 40 px (size-10 ve h-10) sınıfındadır', () => {
    const w = mountPager({ page: 2 })

    expect(prev(w).classes()).toContain('h-10')
    expect(next(w).classes()).toContain('h-10')
    expect(w.get('[data-slot="pagination-item"]').classes()).toContain('size-10')
  })
})
