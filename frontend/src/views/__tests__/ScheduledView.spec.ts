import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ScheduledView from '../ScheduledView.vue'
import { deferred, errorResponse, jsonResponse } from '@/__tests__/helpers'
import { scheduledPageBody, scheduledRow } from '@/__tests__/scheduled-helpers'
import type { ScheduledCampaign } from '@/api/scheduled'

const fetchMock = vi.fn<typeof fetch>()

// Planlanan zamanlar bu "şimdi"ye göre verilir. Yalnızca zamanlayıcılar ve Date sahte: söz zincirleri
// ve flushPromises gerçek kalır.
const NOW = Date.parse('2026-11-21T09:00:00Z')
const MIN = 60_000

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval', 'Date'] })
  vi.setSystemTime(NOW)
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString()

/**
 * Sunucu: sayfa başına 10 kayıt döndürür; `server_time` HER yanıtta o anki (sahte) saattir.
 * `skew`: sunucu saatinin cihaz saatinden farkı.
 */
function serve(all: ScheduledCampaign[], skew = 0) {
  fetchMock.mockImplementation((input) => {
    const page = Number(new URL(String(input), 'http://x').searchParams.get('page'))
    const items = all.slice((page - 1) * 10, page * 10)
    return Promise.resolve(
      jsonResponse(
        200,
        scheduledPageBody({
          items,
          total: all.length,
          page,
          server_time: new Date(Date.now() + skew).toISOString(),
        }),
      ),
    )
  })
}

const urls = () => fetchMock.mock.calls.map(([u]) => String(u))
const rows = (w: VueWrapper) => w.findAll('[data-testid="scheduled-row"]')
const countdown = (w: VueWrapper, index = 0) =>
  rows(w)[index]!.get('[data-testid="scheduled-countdown"]').text()

async function mountView() {
  const w = mount(ScheduledView, { global: { stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return w
}

async function advance(ms: number) {
  await vi.advanceTimersByTimeAsync(ms)
  await flushPromises()
}

describe('ScheduledView', () => {
  describe('liste', () => {
    it('ilk sayfayı ister ve brief, başlangıç saati ve kuyruk sayısıyla satırları gösterir', async () => {
      serve([
        scheduledRow({ id: 'a', brief: 'Yakın kampanya', scheduled_at: at(90 * MIN), queued: 191 }),
        scheduledRow({
          id: 'b',
          brief: 'Uzak kampanya',
          scheduled_at: at(26 * 60 * MIN),
          queued: 7,
        }),
      ])
      const w = await mountView()

      expect(urls()).toEqual(['/api/scheduled?page=1'])
      expect(rows(w)).toHaveLength(2)
      const first = rows(w)[0]!
      expect(first.get('[data-testid="scheduled-brief"]').text()).toBe('Yakın kampanya')
      expect(first.get('[data-testid="scheduled-queued"]').text()).toContain('191 mesaj kuyrukta')
      expect(first.get('[data-testid="scheduled-start"]').text()).toMatch(/\d{2}:\d{2}/)
      expect(w.get('[data-testid="scheduled-total"]').text()).toBe('2 kampanya bekliyor')
    })

    it('sayfa başlığında sidebar ile aynı ikon karosu vardır', async () => {
      serve([scheduledRow()])
      const w = await mountView()

      expect(w.find('[data-testid="page-header-icon"] svg').exists()).toBe(true)
    })

    it('başlıkta sayfa adı ve saat dilimi uyarısı vardır; en erken zaman notu görünür', async () => {
      serve([scheduledRow()])
      const w = await mountView()

      expect(w.get('h1').text()).toBe('Zamanlanmış gönderimler')
      expect(w.get('[data-testid="page-description"]').text()).toContain('saat dilimindedir')
      expect(w.get('[data-testid="scheduled-note"]').text()).toContain('en erken zamandır')
    })

    it('her satır kampanyanın gönderim takibine açılır', async () => {
      serve([scheduledRow({ id: 'abc-123' })])
      const w = await mountView()

      const link = rows(w)[0]!.findComponent(RouterLinkStub)
      expect(link.props('to')).toBe('/campaigns/abc-123/delivery')
      expect(link.text()).toContain('Gönderimi aç')
    })

    it('brief HTML olarak yorumlanmaz', async () => {
      serve([scheduledRow({ brief: '<b>kalın</b><img src=x onerror=alert(1)>' })])
      const w = await mountView()

      const row = rows(w)[0]!
      expect(row.find('b').exists()).toBe(false)
      expect(row.find('img').exists()).toBe(false)
      expect(row.get('[data-testid="scheduled-brief"]').text()).toContain('<b>kalın</b>')
    })
  })

  describe('kalan süre (sunucu saatine göre)', () => {
    it('kalan süreyi "… sonra başlayacak" olarak yazar', async () => {
      serve([scheduledRow({ scheduled_at: at(90 * MIN) })])
      const w = await mountView()

      expect(countdown(w)).toBe('1 sa 30 dk sonra başlayacak')
      expect(rows(w)[0]!.attributes('data-due')).toBe('false')
    })

    it('cihaz saati yanlış olsa da sunucu saatine göre doğru gösterir', async () => {
      // Sunucu cihazdan 3 saat ileride; planlanan zaman SUNUCUYA göre 90 dk sonra.
      const skew = 3 * 60 * MIN
      serve([scheduledRow({ scheduled_at: at(skew + 90 * MIN) })], skew)
      const w = await mountView()

      expect(countdown(w)).toBe('1 sa 30 dk sonra başlayacak')
    })

    it("15 sn'de bir tazelenir: süre azaldıkça metin güncellenir", async () => {
      serve([scheduledRow({ scheduled_at: at(2 * MIN + 30_000) })])
      const w = await mountView()
      expect(countdown(w)).toBe('2 dk sonra başlayacak')

      await advance(60_000)

      expect(countdown(w)).toBe('1 dk sonra başlayacak')
    })

    it('süre dolunca "Zamanı geldi, işleniyor" der (sunucu yenilemesini beklemeden)', async () => {
      serve([scheduledRow({ scheduled_at: at(20_000) })])
      // Sunucu yenilemesi gelmesin: yalnızca sayacın sıfırı geçmesi sınanır.
      const w = await mountView()
      fetchMock.mockReturnValue(deferred<Response>().promise)

      await advance(30_000) // iki tik: 15. sn'de 5 sn kalır, 30. sn'de süre dolmuştur

      expect(countdown(w)).toBe('Zamanı geldi, işleniyor')
      expect(rows(w)[0]!.attributes('data-due')).toBe('true')
    })

    it('sunucu "zamanı geldi" derse (worker kapalı) süre ileride görünse bile bunu gösterir', async () => {
      serve([scheduledRow({ scheduled_at: at(5 * MIN), due: true })])
      const w = await mountView()

      expect(countdown(w)).toBe('Zamanı geldi, işleniyor')
    })
  })

  describe('yenileme', () => {
    it("30 sn'de bir sessizce yeniden okur: yükleme iskeleti çıkmaz, satırlar yerinde kalır", async () => {
      serve([scheduledRow({ id: 'a' })])
      const w = await mountView()
      expect(urls()).toHaveLength(1)

      const pending = deferred<Response>()
      fetchMock.mockReturnValue(pending.promise)
      await advance(30_000)

      expect(urls()).toHaveLength(2)
      expect(w.find('[data-testid="scheduled-loading"]').exists()).toBe(false)
      expect(rows(w)).toHaveLength(1)
    })

    it('gönderimi başlayan kampanya yenilemede listeden çıkar', async () => {
      serve([scheduledRow({ id: 'a' }), scheduledRow({ id: 'b' })])
      const w = await mountView()
      expect(rows(w)).toHaveLength(2)

      serve([scheduledRow({ id: 'b' })])
      await advance(30_000)

      expect(rows(w)).toHaveLength(1)
      expect(w.get('[data-testid="scheduled-total"]').text()).toBe('1 kampanya bekliyor')
    })

    it('sessiz yenileme başarısız olursa eski liste kalır ve hata paneli çıkmaz', async () => {
      serve([scheduledRow({ id: 'a' })])
      const w = await mountView()

      fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))
      await advance(30_000)

      expect(rows(w)).toHaveLength(1)
      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
    })

    it('"Yenile" düğmesi geçerli sayfayı hemen yeniden okur', async () => {
      serve([scheduledRow()])
      const w = await mountView()

      await w.get('[data-testid="scheduled-refresh"]').trigger('click')
      await flushPromises()

      expect(urls()).toEqual(['/api/scheduled?page=1', '/api/scheduled?page=1'])
    })

    it('bileşen kaldırılınca zamanlayıcılar durur ve süren istek iptal edilir', async () => {
      serve([scheduledRow()])
      const w = await mountView()
      const pending = deferred<Response>()
      fetchMock.mockReturnValue(pending.promise)
      await advance(30_000)
      const signal = fetchMock.mock.calls[fetchMock.mock.calls.length - 1]![1]?.signal

      w.unmount()
      const calls = fetchMock.mock.calls.length
      await advance(120_000)

      expect(signal?.aborted).toBe(true)
      expect(fetchMock.mock.calls.length).toBe(calls)
    })
  })

  describe('boş, yükleme ve hata', () => {
    it('hiç planlı kampanya yoksa açıklama ve "Yeni kampanya" bağlantısı gösterir', async () => {
      serve([])
      const w = await mountView()

      const empty = w.get('[data-testid="scheduled-empty"]')
      expect(empty.text()).toContain('Zamanlanmış gönderim yok')
      expect(empty.text()).toContain('tarih ve saat seç')
      expect(empty.findComponent(RouterLinkStub).props('to')).toBe('/')
      expect(w.find('[data-testid="scheduled-list"]').exists()).toBe(false)
      // Boşken "0 kampanya bekliyor" yazılmaz: boş durum zaten söyler.
      expect(w.find('[data-testid="scheduled-total"]').exists()).toBe(false)
      expect(w.text()).not.toContain('0 kampanya bekliyor')
      expect(w.find('[data-testid="scheduled-refresh"]').exists()).toBe(true)
    })

    it('yanıt gelene kadar iskelet gösterir', async () => {
      fetchMock.mockReturnValue(deferred<Response>().promise)
      const w = await mountView()

      const loading = w.get('[data-testid="scheduled-loading"]')
      expect(loading.attributes('role')).toBe('status')
      expect(loading.text()).toContain('yükleniyor')
      expect(rows(w)).toHaveLength(0)
    })

    it('hata panelinde başlık ve "Yeniden dene" çıkar; deneyince listeyi yükler', async () => {
      fetchMock.mockResolvedValueOnce(errorResponse(500, 'INTERNAL'))
      const w = await mountView()

      expect(w.get('[data-testid="error-panel"]').text()).toContain(
        'Zamanlanmış gönderimler yüklenemedi.',
      )
      serve([scheduledRow()])
      await w.get('[data-testid="error-retry"]').trigger('click')
      await flushPromises()

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
      expect(rows(w)).toHaveLength(1)
    })

    it('ağ hatasında açıklama ve yeniden deneme sunar', async () => {
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
      const w = await mountView()

      expect(w.get('[data-testid="error-panel"]').text()).toContain('Sunucuya ulaşılamadı.')
      expect(w.find('[data-testid="error-retry"]').exists()).toBe(true)
    })

    it('geçersiz yanıtı beklenmeyen yanıt olarak gösterir (yarım veri çizmez)', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { items: 'x' }))
      const w = await mountView()

      expect(w.get('[data-testid="error-panel"]').text()).toContain(
        'Sunucudan beklenmeyen bir yanıt alındı.',
      )
      expect(rows(w)).toHaveLength(0)
    })
  })

  describe('sayfalama', () => {
    const many = Array.from({ length: 25 }, (_, i) =>
      scheduledRow({ id: `c${i}`, brief: `Kampanya ${i}`, scheduled_at: at((i + 1) * 10 * MIN) }),
    )

    it("10'arlı sayfalar sunar; ikinci sayfayı ister", async () => {
      serve(many)
      const w = await mountView()
      expect(rows(w)).toHaveLength(10)

      await w.get('[aria-label="Sonraki sayfa"]').trigger('click')
      await flushPromises()

      expect(urls().slice(-1)).toEqual(['/api/scheduled?page=2'])
      expect(rows(w)[0]!.get('[data-testid="scheduled-brief"]').text()).toBe('Kampanya 10')
    })

    it('tek sayfa varsa sayfalama düğmesi çıkmaz', async () => {
      serve(many.slice(0, 3))
      const w = await mountView()

      expect(w.find('[data-slot="pagination"]').exists()).toBe(false)
    })

    it('liste kısalıp istenen sayfa boş kalırsa son sayfaya geçer', async () => {
      serve(many)
      const w = await mountView()
      await w.get('[aria-label="Sayfa 3"]').trigger('click')
      await flushPromises()
      serve(many.slice(0, 12)) // kampanyalar başladı: sayfa 3 artık yok

      await w.get('[data-testid="scheduled-refresh"]').trigger('click')
      await flushPromises()

      expect(urls().slice(-2)).toEqual(['/api/scheduled?page=3', '/api/scheduled?page=2'])
      expect(rows(w)).toHaveLength(2)
      expect(w.get('[data-testid="scheduled-total"]').text()).toBe('12 kampanya bekliyor')
    })
  })
})
