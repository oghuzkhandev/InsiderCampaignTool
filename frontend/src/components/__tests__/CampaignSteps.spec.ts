import { describe, expect, it } from 'vitest'
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

  it('tamamlanan adımda tik, diğerlerinde sıra numarası görünür', () => {
    const w = mountSteps('draft')
    const [audience, content] = w.findAll('li')
    expect(audience?.find('svg').exists()).toBe(true)
    expect(content?.find('svg').exists()).toBe(false)
    expect(content?.text()).toContain('2.')
  })

  it('listeye erişilebilir bir ad verir', () => {
    expect(mountSteps(null).get('ol').attributes('aria-label')).toBe('Kampanya adımları')
  })
})
