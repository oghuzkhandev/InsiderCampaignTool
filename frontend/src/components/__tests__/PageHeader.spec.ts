import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { UsersRoundIcon } from '@lucide/vue'
import PageHeader from '../PageHeader.vue'

describe('PageHeader', () => {
  it('başlığı h1 olarak ve açıklamayı gösterir', () => {
    const w = mount(PageHeader, { props: { title: 'Kampanya', description: 'Kısa açıklama.' } })
    expect(w.get('h1').text()).toBe('Kampanya')
    expect(w.get('[data-testid="page-description"]').text()).toBe('Kısa açıklama.')
  })

  it('ikon verilirse başlığın solunda süs olarak çizilir (ekran okuyucudan gizli)', () => {
    const w = mount(PageHeader, { props: { title: 'Kitleler', icon: UsersRoundIcon } })

    const tile = w.get('[data-testid="page-header-icon"]')
    expect(tile.attributes('aria-hidden')).toBe('true')
    expect(tile.find('svg').exists()).toBe(true)
    // Başlık metni ikondan etkilenmez ve tek h1 vardır.
    expect(w.get('h1').text()).toBe('Kitleler')
    expect(w.findAll('h1')).toHaveLength(1)
    // İkon başlıktan önce gelir.
    expect(
      tile.element.compareDocumentPosition(w.get('h1').element) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })

  it('ikon verilmezse karo çizilmez', () => {
    const w = mount(PageHeader, { props: { title: 'Kampanya' } })

    expect(w.find('[data-testid="page-header-icon"]').exists()).toBe(false)
  })

  it('başlık ayracı her zaman vardır', () => {
    expect(
      mount(PageHeader, { props: { title: 'X' } })
        .find('[data-testid="page-header-separator"]')
        .exists(),
    ).toBe(true)
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
