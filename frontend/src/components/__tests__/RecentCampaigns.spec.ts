import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import RecentCampaigns from '../RecentCampaigns.vue'
import { deferred, errorResponse, jsonResponse } from '@/__tests__/helpers'

const fetchMock = vi.fn<typeof fetch>()
let mounted: VueWrapper[] = []

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  mounted.forEach((w) => w.unmount())
  mounted = []
  vi.unstubAllGlobals()
})

const row = (n: number, status = 'draft', extra: Record<string, unknown> = {}) => ({
  id: `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`,
  brief: `Brief ${n}`,
  status,
  created_at: `2026-11-19T${String(10 + n).padStart(2, '0')}:30:00Z`,
  ...extra,
})
const list = (...rows: unknown[]) => jsonResponse(200, { campaigns: rows })

async function mountList(reloadKey: string | null = null): Promise<VueWrapper> {
  const w = mount(RecentCampaigns, {
    props: { reloadKey },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
  mounted.push(w)
  await flushPromises()
  return w
}

const rowsOf = (w: VueWrapper) => w.findAll('[data-testid="recent-row"]')
const linkTo = (w: VueWrapper, i: number) => w.findAllComponents(RouterLinkStub)[i]?.props('to')

describe('yükleme', () => {
  it('açılınca varsayılan sınırla (10) listeyi ister', async () => {
    fetchMock.mockResolvedValue(list(row(2), row(1)))
    await mountList()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/campaigns?limit=10')
    expect(init?.method).toBeUndefined() // GET
  })

  it('ilk yüklemede iskelet gösterir, sonra satırları', async () => {
    const pending = deferred<Response>()
    fetchMock.mockReturnValue(pending.promise)
    const w = mount(RecentCampaigns, { global: { stubs: { RouterLink: RouterLinkStub } } })
    mounted.push(w)
    await flushPromises()

    expect(w.get('[data-testid="recent-loading"]').text()).toContain('yükleniyor')
    expect(w.find('[data-testid="recent-list"]').exists()).toBe(false)

    pending.resolve(list(row(1)))
    await flushPromises()
    expect(w.find('[data-testid="recent-loading"]').exists()).toBe(false)
    expect(rowsOf(w)).toHaveLength(1)
  })
})

describe('satırlar', () => {
  it('sunucunun verdiği sırayla (en yeni başta); her satırda brief, durum ve tarih vardır', async () => {
    fetchMock.mockResolvedValue(list(row(3, 'completed'), row(2, 'sending'), row(1, 'draft')))
    const w = await mountList()

    const briefs = w.findAll('[data-testid="recent-brief"]').map((b) => b.text())
    expect(briefs).toEqual(['Brief 3', 'Brief 2', 'Brief 1'])
    expect(w.findAll('[data-testid="recent-status"]').map((s) => s.text())).toEqual([
      'Tamamlandı',
      'Gönderiliyor',
      'Taslak',
    ])
    const times = w.findAll('time')
    expect(times).toHaveLength(3)
    expect(times[0]?.attributes('datetime')).toBe('2026-11-19T13:30:00Z')
    // Türkçe biçimli, yıl ve saat içerir.
    expect(times[0]?.text()).toMatch(/2026/)
    expect(times[0]?.text()).toMatch(/\d{2}[:.]\d{2}/)
  })

  it('kitle sayısı gösterilmez (şimdilik)', async () => {
    fetchMock.mockResolvedValue(
      list(row(1, 'draft', { audience: { total: 190, sms: 140, email: 150 } })),
    )
    const w = await mountList()
    expect(w.text()).not.toContain('190')
    expect(w.text()).not.toMatch(/kitle/i)
  })

  it('taslak kampanya kampanya ekranına açılır', async () => {
    fetchMock.mockResolvedValue(list(row(1, 'draft')))
    const w = await mountList()

    expect(linkTo(w, 0)).toBe(`/campaigns/${row(1).id}`)
    expect(w.get('[data-testid="recent-target"]').text()).toBe('Kampanyayı aç')
  })

  it('satırın nereye açıldığı ekran okuyucuya yazılır ve ipucu olarak da verilir', async () => {
    fetchMock.mockResolvedValue(list(row(2, 'sending'), row(1, 'draft')))
    const w = await mountList()

    const target = w.get('[data-testid="recent-target"]')
    expect(target.classes()).toContain('sr-only')
    const links = w.findAllComponents(RouterLinkStub)
    expect(links.map((l) => l.attributes('title'))).toEqual(['Gönderimi izle', 'Kampanyayı aç'])
  })

  it('durum rozeti simge taşır; simge ekran okuyucudan gizlidir, metin kalır', async () => {
    fetchMock.mockResolvedValue(list(row(1, 'completed')))
    const w = await mountList()

    const badge = w.get('[data-testid="recent-status"]')
    expect(badge.find('svg').attributes('aria-hidden')).toBe('true')
    expect(badge.text()).toBe('Tamamlandı')
  })

  it('onaylanmış, gönderilen ve tamamlanan kampanya gönderim takibine açılır', async () => {
    fetchMock.mockResolvedValue(list(row(3, 'approved'), row(2, 'sending'), row(1, 'completed')))
    const w = await mountList()

    for (const [i, n] of [3, 2, 1].entries()) {
      expect(linkTo(w, i), `satır ${n}`).toBe(`/campaigns/${row(n).id}/delivery`)
    }
    for (const label of w.findAll('[data-testid="recent-target"]')) {
      expect(label.text()).toBe('Gönderimi izle')
    }
  })

  it('bilinmeyen durum gösterilir ve kampanya ekranına açılır', async () => {
    fetchMock.mockResolvedValue(list(row(1, 'archived')))
    const w = await mountList()

    expect(w.get('[data-testid="recent-status"]').text()).toBe('archived')
    expect(linkTo(w, 0)).toBe(`/campaigns/${row(1).id}`)
  })

  it('brief metnini HTML olarak yorumlamaz', async () => {
    fetchMock.mockResolvedValue(list(row(1, 'draft', { brief: '<img src=x onerror=alert(1)>' })))
    const w = await mountList()

    expect(w.find('img').exists()).toBe(false)
    expect(w.get('[data-testid="recent-brief"]').text()).toBe('<img src=x onerror=alert(1)>')
  })

  it('uzun ve boşluksuz brief kırılır (taşmaz) ve en çok iki satırda gösterilir', async () => {
    fetchMock.mockResolvedValue(list(row(1, 'draft', { brief: 'x'.repeat(1000) })))
    const w = await mountList()

    const classes = w.get('[data-testid="recent-brief"]').classes()
    expect(classes).toContain('wrap-anywhere')
    expect(classes).toContain('line-clamp-2')
  })
})

describe('boş ve hata durumları', () => {
  it('hiç kampanya yoksa yönlendirir', async () => {
    fetchMock.mockResolvedValue(list())
    const w = await mountList()

    expect(w.get('[data-testid="recent-empty"]').text()).toContain('Henüz kampanya yok')
    expect(w.find('[data-testid="recent-list"]').exists()).toBe(false)
  })

  it('yüklenemezse hata ve "Yeniden dene" gösterir; yeniden deneyince listeyi yükler', async () => {
    fetchMock.mockResolvedValueOnce(errorResponse(500, 'INTERNAL'))
    const w = await mountList()

    const panel = w.get('[data-testid="error-panel"]')
    expect(panel.text()).toContain('Son kampanyalar yüklenemedi')
    expect(w.find('[data-testid="recent-list"]').exists()).toBe(false)

    fetchMock.mockResolvedValueOnce(list(row(1)))
    await w.get('[data-testid="error-retry"]').trigger('click')
    await flushPromises()

    expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
    expect(rowsOf(w)).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('ağ hatasında sunucuya ulaşılamadığını söyler', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('failed'))
    const w = await mountList()
    expect(w.get('[data-testid="error-panel"]').text()).toContain('Sunucuya ulaşılamadı')
  })

  it.each([
    ['gövde dizi değil', { campaigns: 'x' }],
    ['alan eksik', { campaigns: [{ id: 'a', brief: 'b', status: 'draft' }] }],
    [
      'tarih geçersiz',
      { campaigns: [{ id: 'a', brief: 'b', status: 'draft', created_at: 'dün' }] },
    ],
    ['kampanyalar yok', {}],
  ])('beklenmeyen yanıtı hata sayar: %s', async (_name, body) => {
    fetchMock.mockResolvedValue(jsonResponse(200, body))
    const w = await mountList()
    expect(w.get('[data-testid="error-panel"]').text()).toContain('beklenmeyen bir yanıt')
  })
})

describe('yenileme', () => {
  it('reloadKey değişince listeyi yeniden okur ve yeni satır en üstte görünür', async () => {
    fetchMock.mockResolvedValueOnce(list(row(1)))
    const w = await mountList(null)
    expect(rowsOf(w)).toHaveLength(1)

    fetchMock.mockResolvedValueOnce(list(row(2), row(1)))
    await w.setProps({ reloadKey: row(2).id })
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(w.findAll('[data-testid="recent-brief"]').map((b) => b.text())).toEqual([
      'Brief 2',
      'Brief 1',
    ])
  })

  it('yenilenirken eski satırlar görünür kalır (iskelete dönmez)', async () => {
    fetchMock.mockResolvedValueOnce(list(row(1)))
    const w = await mountList(null)

    const pending = deferred<Response>()
    fetchMock.mockReturnValueOnce(pending.promise)
    await w.setProps({ reloadKey: 'yeni' })

    expect(rowsOf(w)).toHaveLength(1)
    expect(w.find('[data-testid="recent-loading"]').exists()).toBe(false)
    pending.resolve(list(row(2), row(1)))
    await flushPromises()
    expect(rowsOf(w)).toHaveLength(2)
  })

  it('geç gelen eski yanıt yeni listeyi ezmez ve eski istek iptal edilir', async () => {
    const slow = deferred<Response>()
    fetchMock.mockReturnValueOnce(slow.promise)
    const w = mount(RecentCampaigns, {
      props: { reloadKey: null },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    mounted.push(w)
    await flushPromises()
    const firstSignal = fetchMock.mock.calls[0]![1]?.signal

    fetchMock.mockResolvedValueOnce(list(row(2)))
    await w.setProps({ reloadKey: 'yeni' })
    await flushPromises()
    expect(firstSignal?.aborted).toBe(true)

    slow.resolve(list(row(1)))
    await flushPromises()
    expect(w.findAll('[data-testid="recent-brief"]').map((b) => b.text())).toEqual(['Brief 2'])
  })

  it('bileşen kaldırılınca süren isteği iptal eder', async () => {
    fetchMock.mockReturnValue(deferred<Response>().promise)
    const w = mount(RecentCampaigns, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()

    w.unmount()

    expect(fetchMock.mock.calls[0]![1]?.signal?.aborted).toBe(true)
  })
})

describe('erişilebilirlik', () => {
  it('adlandırılmış bir bölge ve liste olarak sunulur', async () => {
    fetchMock.mockResolvedValue(list(row(1)))
    const w = await mountList()

    expect(w.get('[data-testid="recent-campaigns"]').attributes('aria-labelledby')).toBe(
      'recent-title',
    )
    expect(w.get('h2#recent-title').text()).toBe('Son kampanyalar')
    expect(w.findAll('ul > li')).toHaveLength(1)
  })
})
