import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CampaignSteps from '../CampaignSteps.vue'
import { stepStatuses } from '@/campaign/steps'

const mountSteps = (status: string | null, contentReady = false) =>
  mount(CampaignSteps, { props: { statuses: stepStatuses({ status, contentReady }) } })

describe('CampaignSteps', () => {
  it('dört adımı sırayla ve adlarıyla gösterir', () => {
    const w = mountSteps('draft')
    const items = w.findAll('li')
    expect(items).toHaveLength(4)
    expect(items.map((li) => li.text().replace(/\s+/g, ' '))).toEqual([
      expect.stringContaining('Kitle'),
      expect.stringContaining('İçerik'),
      expect.stringContaining('Onay'),
      expect.stringContaining('Gönderim'),
    ])
  })

  it('yalnızca şu anki adımı aria-current="step" ile işaretler', () => {
    const w = mountSteps('draft', true)
    const current = w.findAll('li[aria-current="step"]')
    expect(current).toHaveLength(1)
    expect(current[0]?.attributes('data-step')).toBe('approval')
  })

  it('hiçbir adım şu anki değilse (tamamlandı) aria-current yoktur', () => {
    const w = mountSteps('completed')
    expect(w.find('[aria-current]').exists()).toBe(false)
  })

  it('durumu ekran okuyucuya metinle de bildirir (yalnızca renk değil)', () => {
    const w = mountSteps('draft')
    const text = w.findAll('li').map((li) => li.get('.sr-only').text())
    expect(text).toEqual(['(tamamlandı)', '(şu anki adım)', '(bekliyor)', '(bekliyor)'])
  })

  it('tamamlanan adımda tik, diğerlerinde yuvarlak içinde sıra numarası görünür', () => {
    const w = mountSteps('draft')
    const circles = w.findAll('[data-testid="step-circle"]')
    expect(circles.map((c) => c.text())).toEqual(['', '2', '3', '4'])
    expect(circles[0]?.find('svg').exists()).toBe(true)
    expect(circles[1]?.find('svg').exists()).toBe(false)
  })

  it('her durumun kendi rengi vardır: biten yeşil, şu anki ana renk, bekleyen soluk', () => {
    const w = mountSteps('draft')
    const [done, current, upcoming] = w.findAll('[data-testid="step-circle"]')
    expect(done?.classes()).toContain('bg-emerald-600')
    expect(current?.classes()).toContain('bg-primary')
    expect(upcoming?.classes()).toContain('bg-background')
    expect(upcoming?.classes()).toContain('text-muted-foreground')
  })

  it('yalnızca şu anki adımda nabız halkası vardır; daire büyütülmez (kenardan taşmaz)', () => {
    const w = mountSteps('draft')
    const items = w.findAll('li')
    expect(items.map((li) => li.find('[data-testid="step-pulse"]').exists())).toEqual([
      false,
      true,
      false,
      false,
    ])
    const classes = w.findAll('[data-testid="step-circle"]').flatMap((c) => c.classes())
    expect(classes.some((c) => c.startsWith('scale-'))).toBe(false)
  })

  it("liste, halkanın taşan 4 px'i için iç boşluk taşır (kenarla hizalı)", () => {
    expect(mountSteps('draft').get('ol').classes()).toContain('px-1')
  })

  it('her adımın anlamına uygun, ekran okuyucudan gizli bir ikonu vardır ve ikonlar birbirinden farklıdır', () => {
    const w = mountSteps('draft')
    const icons = w.findAll('[data-testid="step-icon"]')
    expect(icons).toHaveLength(4)
    expect(icons.every((i) => i.attributes('aria-hidden') === 'true')).toBe(true)
    expect(new Set(icons.map((i) => i.html())).size).toBe(4)
  })

  it('kısa durum yazısı görünür metindir ama ekran okuyucudan gizlidir (tek kaynak sr-only)', () => {
    const w = mountSteps('draft')
    const captions = w.findAll('[data-testid="step-caption"]')
    expect(captions.map((c) => c.text())).toEqual([
      'Tamamlandı',
      'Devam ediyor',
      'Bekliyor',
      'Bekliyor',
    ])
    expect(captions.every((c) => c.attributes('aria-hidden') === 'true')).toBe(true)
  })

  it('çizgi, yalnızca biten adımdan sonra dolu işaretlenir ve son adımın çizgisi yoktur', () => {
    const w = mountSteps('approved')
    const connectors = w.findAll('[data-testid="step-connector"]')
    expect(connectors).toHaveLength(3)
    expect(connectors.map((c) => c.attributes('data-filled'))).toEqual(['true', 'true', 'true'])

    const draft = mountSteps('draft')
    expect(
      draft.findAll('[data-testid="step-connector"]').map((c) => c.attributes('data-filled')),
    ).toEqual(['true', 'false', 'false'])
  })

  it('çizgi açılışta boş başlar, kısa süre sonra dolar (yumuşak geçiş)', async () => {
    vi.useFakeTimers()
    try {
      const w = mountSteps('approved')
      const fill = () => w.get('[data-testid="step-connector"] > span')
      expect(fill().classes()).toContain('scale-x-0')

      await vi.advanceTimersByTimeAsync(100)

      expect(fill().classes()).toContain('scale-x-100')
    } finally {
      vi.useRealTimers()
    }
  })

  it('durum değişince (şu anki adım biter) çizgi dolar ve daire yeşile geçer', async () => {
    vi.useFakeTimers()
    try {
      const w = mountSteps('draft')
      await vi.advanceTimersByTimeAsync(100)
      const connector = () => w.findAll('[data-testid="step-connector"]')[1]!
      expect(connector().get('span').classes()).toContain('scale-x-0')

      await w.setProps({ statuses: stepStatuses({ status: 'draft', contentReady: true }) })
      await w.setProps({ statuses: stepStatuses({ status: 'approved', contentReady: true }) })

      expect(connector().get('span').classes()).toContain('scale-x-100')
      expect(w.findAll('[data-testid="step-circle"]')[1]?.classes()).toContain('bg-emerald-600')
    } finally {
      vi.useRealTimers()
    }
  })

  it('sayaç bileşen kaldırılınca temizlenir (kalıntı yok)', () => {
    vi.useFakeTimers()
    try {
      const w = mountSteps('draft')
      w.unmount()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      vi.useRealTimers()
    }
  })

  it('listeye erişilebilir bir ad verir', () => {
    expect(mountSteps(null).get('ol').attributes('aria-label')).toBe('Kampanya adımları')
  })
})
