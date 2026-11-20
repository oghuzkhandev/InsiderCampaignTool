import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import App from '../App.vue'
import router from '../router'
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

describe('uygulama kabuğu (üst başlık ve menü)', () => {
  it('marka adı ve beş sayfa bağlantısı görünür; marka ikonu yoktur', async () => {
    const api = stubApi()
    const wrapper = await mountAt('/')
    const brand = wrapper.get('[data-testid="brand-link"]')
    expect(brand.text()).toBe('Insider Campaign Tool')
    expect(brand.attributes('href')).toBe('/overview')
    expect(brand.find('svg').exists()).toBe(false)
    const links = wrapper.get('nav[aria-label="Sayfalar"]').findAll('a')
    expect(links.map((link) => [link.text(), link.attributes('href')])).toEqual([
      ['Genel bakış', '/overview'],
      ['Yeni kampanya', '/'],
      ['Kampanyalar', '/campaigns'],
      ['Kitleler', '/audiences'],
      ['İçerik şablonları', '/templates'],
    ])
    expect(wrapper.findAll('h1')).toHaveLength(1)
    expect(wrapper.find('[data-testid="recent-campaigns"]').exists()).toBe(false)
    expect(api.listCalls()).toHaveLength(0)
  })

  it('marka bağlantısı genel bakışı açar', async () => {
    stubApi()
    const wrapper = await mountAt('/campaigns/abc')
    await wrapper.get('[data-testid="brand-link"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/overview')
    expect(wrapper.get('h1').text()).toBe('Genel bakış')
  })

  it('kampanya ayrıntısında Kampanyalar menüsü etkin kalır', async () => {
    stubApi()
    const wrapper = await mountAt('/campaigns/abc')
    const active = wrapper.get('nav[aria-label="Sayfalar"] a[aria-current="page"]')
    expect(active.text()).toBe('Kampanyalar')
  })
})
