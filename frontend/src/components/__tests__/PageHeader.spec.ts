import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import PageHeader from '../PageHeader.vue'

describe('PageHeader', () => {
  it('başlığı h1 olarak ve açıklamayı gösterir', () => {
    const w = mount(PageHeader, { props: { title: 'Kampanya', description: 'Kısa açıklama.' } })
    expect(w.get('h1').text()).toBe('Kampanya')
    expect(w.get('[data-testid="page-description"]').text()).toBe('Kısa açıklama.')
  })

  it('açıklama verilmezse açıklama öğesi yoktur', () => {
    const w = mount(PageHeader, { props: { title: 'Kampanya' } })
    expect(w.find('[data-testid="page-description"]').exists()).toBe(false)
  })

  it('rozet ve adım göstergesi yuvalarını yerleştirir', () => {
    const w = mount(PageHeader, {
      props: { title: 'Kampanya' },
      slots: {
        badge: '<span data-testid="badge">Taslak</span>',
        default: '<ol data-testid="steps"></ol>',
      },
    })
    expect(w.get('[data-testid="badge"]').text()).toBe('Taslak')
    expect(w.find('[data-testid="steps"]').exists()).toBe(true)
    // Rozet başlıkla aynı satırdadır.
    expect(
      w.get('h1').element.parentElement?.contains(w.get('[data-testid="badge"]').element),
    ).toBe(true)
  })
})
