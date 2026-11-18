import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import App from '../App.vue'
import router from '../router'

describe('App', () => {
  it('ana adreste kampanya önizleme ekranını gösterir', async () => {
    await router.push('/')
    await router.isReady()

    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe('Kampanya önizleme')
    expect(wrapper.find('textarea#brief').exists()).toBe(true)
    wrapper.unmount() // ortak router'a bağlı kalmasın
  })

  it("/campaigns/:id adresinde kampanya ekranını açar ve kimliği API'ye iletir", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: 'abc',
          brief: 'Örnek brief',
          status: 'draft',
          segment: { all: [{ field: 'city', op: 'eq', value: 'İstanbul' }] },
          audience: { total: 1, sms: 1, email: 1 },
          created_at: '2026-11-17T17:57:48Z',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    try {
      await router.push('/campaigns/abc')
      await router.isReady()

      const wrapper = mount(App, { global: { plugins: [router] } })
      await flushPromises()

      expect(wrapper.get('h1').text()).toBe('Kampanya')
      expect(wrapper.text()).toContain('Örnek brief')
      expect(fetchMock).toHaveBeenCalledTimes(1)
      expect(fetchMock.mock.calls[0]![0]).toBe('/api/campaigns/abc')
      wrapper.unmount()
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
