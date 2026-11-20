import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import App from '../App.vue'
import router from '../router'
import { notifyCampaignsChanged } from '@/campaign/recent'
import { jsonResponse } from '@/__tests__/helpers'

const detail = {
  id: 'abc',
  brief: 'Örnek brief',
  status: 'draft',
  segment: { all: [{ field: 'city', op: 'eq', value: 'İstanbul' }] },
  audience: { total: 1, sms: 1, email: 1 },
  created_at: '2026-11-17T17:57:48Z',
}

const summary = (id: string, brief: string, status = 'draft') => ({
  id,
  brief,
  status,
  created_at: '2026-11-19T21:00:00Z',
})

/**
 * Kenar çubuğu "Son kampanyalar" listesini (`/api/campaigns?limit=...`), ekran ise kampanya
 * ayrıntısını (`/api/campaigns/abc`) ister. İstekleri adrese göre yanıtlar ve ayrı sayar.
 */
function stubApi(listBody: unknown = { campaigns: [summary('abc', 'Kayıtlı brief')] }) {
  const fetchMock = vi.fn<typeof fetch>().mockImplementation(async (input) => {
    const url = String(input)
    if (url.startsWith('/api/campaigns?')) return jsonResponse(200, listBody)
    return jsonResponse(200, detail)
  })
  vi.stubGlobal('fetch', fetchMock)
  const urls = () => fetchMock.mock.calls.map(([input]) => String(input))
  return {
    fetchMock,
    listCalls: () => urls().filter((u) => u.startsWith('/api/campaigns?')),
    otherCalls: () => urls().filter((u) => !u.startsWith('/api/campaigns?')),
  }
}

let mounted: VueWrapper | null = null
async function mountAt(path: string): Promise<VueWrapper> {
  await router.push(path)
  await router.isReady()
  mounted = mount(App, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return mounted
}

afterEach(() => {
  mounted?.unmount() // ortak router'a ve belgeye bağlı kalmasın
  mounted = null
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('ana adreste kampanya önizleme ekranını gösterir', async () => {
    stubApi()
    const wrapper = await mountAt('/')

    expect(wrapper.get('h1').text()).toBe('Kampanya önizleme')
    expect(wrapper.find('textarea#brief').exists()).toBe(true)
  })

  it("/campaigns/:id adresinde kampanya ekranını açar ve kimliği API'ye iletir", async () => {
    const api = stubApi()
    const wrapper = await mountAt('/campaigns/abc')

    expect(wrapper.get('h1').text()).toBe('Kampanya')
    expect(wrapper.text()).toContain('Örnek brief')
    expect(api.otherCalls()).toEqual(['/api/campaigns/abc'])
  })

  it("/campaigns/:id/delivery adresinde gönderim takibi ekranını açar ve kimliği API'ye iletir", async () => {
    const api = stubApi()
    const wrapper = await mountAt('/campaigns/abc/delivery')

    expect(wrapper.get('h1').text()).toBe('Gönderim takibi')
    expect(wrapper.text()).toContain('Örnek brief')
    expect(router.currentRoute.value.name).toBe('campaign-delivery')
    expect(api.otherCalls()).toEqual(['/api/campaigns/abc'])
    expect(wrapper.text()).toContain('henüz onaylanmadı') // taslak: canlı takibe bağlanmaz
  })

  it('gönderim takibi ekranının geri bağlantısı kampanya ekranına gider', async () => {
    const resolved = router.resolve('/campaigns/abc/delivery')
    expect(resolved.name).toBe('campaign-delivery')
    expect(resolved.params.id).toBe('abc')
    expect(router.resolve({ name: 'campaign', params: { id: 'abc' } }).path).toBe('/campaigns/abc')
  })
})

describe('uygulama kabuğu (üst başlık ve kenar çubuğu)', () => {
  it('her ekranda "Campaign Tool" başlığı ana sayfaya döner; sayfa başlığı (h1) tek kalır', async () => {
    stubApi()
    for (const path of ['/', '/campaigns/abc', '/campaigns/abc/delivery']) {
      const wrapper = await mountAt(path)

      const brand = wrapper.get('[data-testid="brand-link"]')
      expect(brand.text()).toBe('Campaign Tool')
      expect(brand.attributes('href')).toBe('/')
      expect(wrapper.findAll('h1')).toHaveLength(1)

      wrapper.unmount()
      mounted = null
    }
  })

  it('marka bağlantısına tıklamak ana sayfaya götürür', async () => {
    stubApi()
    const wrapper = await mountAt('/campaigns/abc')

    await wrapper.get('[data-testid="brand-link"]').trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/')
  })

  it('kenar çubuğunda "Yeni kampanya" ve son kampanyalar listesi bulunur', async () => {
    const api = stubApi({
      campaigns: [summary('b', 'İkinci brief', 'approved'), summary('a', 'Birinci brief')],
    })
    const wrapper = await mountAt('/')

    const newCampaign = wrapper.get('[data-testid="sidebar-new-campaign"]')
    expect(newCampaign.text()).toBe('Yeni kampanya')
    expect(newCampaign.attributes('href')).toBe('/')

    const list = wrapper.get('[data-testid="recent-campaigns"]')
    expect(list.findAll('[data-testid="recent-brief"]').map((b) => b.text())).toEqual([
      'İkinci brief',
      'Birinci brief',
    ])
    expect(api.listCalls()).toEqual(['/api/campaigns?limit=10'])
  })

  it('liste sayfa içeriğinden bağımsız çalışır: yüklenemese de ekran açılır', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>().mockImplementation(async (input) => {
        if (String(input).startsWith('/api/campaigns?')) {
          return jsonResponse(500, { error: { code: 'INTERNAL', message: 'x', reasons: [] } })
        }
        return jsonResponse(200, detail)
      }),
    )
    const wrapper = await mountAt('/campaigns/abc')

    expect(wrapper.get('[data-testid="recent-campaigns"]').text()).toContain(
      'Son kampanyalar yüklenemedi',
    )
    expect(wrapper.get('h1').text()).toBe('Kampanya')
    expect(wrapper.text()).toContain('Örnek brief')
  })

  it('bir ekran kampanyayı değiştirince (haber verince) liste yeniden okunur', async () => {
    const api = stubApi()
    await mountAt('/')
    expect(api.listCalls()).toHaveLength(1)

    notifyCampaignsChanged()
    await flushPromises()

    expect(api.listCalls()).toHaveLength(2)
  })

  it('başka ekrana geçince liste yeniden okunur', async () => {
    const api = stubApi()
    await mountAt('/')
    expect(api.listCalls()).toHaveLength(1)

    await router.push('/campaigns/abc')
    await flushPromises()

    expect(api.listCalls()).toHaveLength(2)
  })
})
