import { describe, expect, it } from 'vitest'
import { RouterLinkStub, mount } from '@vue/test-utils'
import StepNav from '../StepNav.vue'

const mountNav = (props: Record<string, unknown>) =>
  mount(StepNav, { props, global: { stubs: { RouterLink: RouterLinkStub } } })

describe('StepNav', () => {
  it('Geri ve İleri bağlantılarını Türkçe, hedef adıyla gösterir', () => {
    const w = mountNav({
      back: { label: 'Kampanyaya dön', to: '/campaigns/1' },
      next: { label: 'Yeni kampanya', to: '/' },
    })
    expect(w.get('[data-testid="nav-back"]').text()).toBe('Geri: Kampanyaya dön')
    expect(w.get('[data-testid="nav-next"]').text()).toBe('İleri: Yeni kampanya')
  })

  it('bağlantılar verilen hedefe gider', () => {
    const w = mountNav({
      back: { label: 'A', to: '/geri' },
      next: { label: 'B', to: { name: 'campaign', params: { id: '7' } } },
    })
    const [back, next] = w.findAllComponents(RouterLinkStub)
    expect(back?.props('to')).toBe('/geri')
    expect(next?.props('to')).toEqual({ name: 'campaign', params: { id: '7' } })
  })

  it('hedef yoksa düğme kapalıdır ve nedenini yazar; bağlantı değildir', () => {
    const w = mountNav({ next: { label: 'Gönderimi izle', hint: 'Önce içeriği onaylayın.' } })
    const next = w.get('[data-testid="nav-next-disabled"]')

    expect(next.element.tagName).toBe('BUTTON')
    expect(next.attributes('disabled')).toBeDefined()
    expect(next.text()).toBe('İleri: Gönderimi izle')
    expect(w.find('[data-testid="nav-next"]').exists()).toBe(false)
    expect(w.get('#step-nav-next-hint').text()).toBe('Önce içeriği onaylayın.')
    expect(next.attributes('aria-describedby')).toBe('step-nav-next-hint')
  })

  it('özel metin verilirse "Geri: ..." / "İleri: ..." kalıbı kullanılmaz', () => {
    const w = mountNav({
      back: { label: 'Yeni kampanya', text: '+ Yeni kampanya', to: '/', title: 'Yeni başlatır.' },
    })
    const back = w.get('[data-testid="nav-back"]')
    expect(back.text()).toBe('+ Yeni kampanya')
    expect(back.text()).not.toContain('Geri')
    expect(back.attributes('title')).toBe('Yeni başlatır.')
  })

  it('özel metin kapalı düğmede de kullanılır', () => {
    const w = mountNav({ next: { label: 'X', text: 'Devam', hint: 'Neden.' } })
    expect(w.get('[data-testid="nav-next-disabled"]').text()).toBe('Devam')
  })

  it('açık düğmede de not gösterilir ve düğmeye bağlanır (bilmesi gereken bilgi)', () => {
    const w = mountNav({
      next: { label: 'Yeni kampanya', to: '/', hint: 'Gönderim arka planda devam eder.' },
    })
    expect(w.get('[data-testid="nav-next"]').attributes('aria-describedby')).toBe(
      'step-nav-next-hint',
    )
    expect(w.get('#step-nav-next-hint').text()).toBe('Gönderim arka planda devam eder.')
  })

  it('not verilmezse not öğesi ve aria-describedby yoktur', () => {
    const w = mountNav({ next: { label: 'B', to: '/x' } })
    expect(w.find('#step-nav-next-hint').exists()).toBe(false)
    expect(w.get('[data-testid="nav-next"]').attributes('aria-describedby')).toBeUndefined()
  })

  it('geri hedefi verilmezse Geri gösterilmez (ilk aşama)', () => {
    const w = mountNav({ next: { label: 'İçeriği hazırla', to: '/x' } })
    expect(w.find('[data-testid="nav-back"]').exists()).toBe(false)
    expect(w.find('[data-testid="nav-back-disabled"]').exists()).toBe(false)
    expect(w.text()).not.toContain('Geri')
  })

  it('özel data-testid kullanılabilir', () => {
    const w = mountNav({ next: { label: 'Gönderimi izle', to: '/x', testid: 'delivery-link' } })
    expect(w.find('[data-testid="delivery-link"]').exists()).toBe(true)
  })

  it('adlandırılmış bir gezinti bölgesidir', () => {
    const w = mountNav({ next: { label: 'B', to: '/x' } })
    expect(w.get('nav').attributes('aria-label')).toBe('Adım gezintisi')
  })

  it('yalnızca sticky istenirse dar ekranda alta yapışır', () => {
    const plain = mountNav({ next: { label: 'B', to: '/x' } })
    const sticky = mountNav({ next: { label: 'B', to: '/x' }, sticky: true })
    expect(plain.get('nav').classes()).not.toContain('sticky')
    expect(sticky.get('nav').classes()).toContain('sticky')
    expect(sticky.get('nav').classes()).toContain('bottom-0')
  })
})
