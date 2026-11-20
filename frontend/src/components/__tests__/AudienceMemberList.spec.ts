import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import AudienceMemberList from '../AudienceMemberList.vue'
import {
  audiencePageBody,
  deferred,
  errorResponse,
  jsonResponse,
  memberBody,
  members,
} from '@/__tests__/helpers'

const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

const ID = 'kampanya-1'

function mountList(campaignId = ID) {
  return mount(AudienceMemberList, { props: { campaignId } })
}

/** Sayfa numarasına göre, toplam `total` kişilik 10'arlı bir liste sunar. */
function serve(total: number) {
  fetchMock.mockImplementation((input) => {
    const page = Number(new URL(String(input), 'http://x').searchParams.get('page'))
    const first = (page - 1) * 10
    const count = Math.max(0, Math.min(10, total - first))
    const items = members(count, first + 1)
    return Promise.resolve(jsonResponse(200, audiencePageBody({ page, total, items })))
  })
}

const urls = () => fetchMock.mock.calls.map(([url]) => String(url))
const rows = (w: VueWrapper) => w.findAll('[data-testid="member-row"]')
const names = (w: VueWrapper) =>
  w.findAll('[data-testid="member-name"]').map((cell) => cell.get('span:last-child').text())
const pagerText = (w: VueWrapper) =>
  w.get('[data-testid="audience-members-status"]').text().replace(/\s+/g, ' ')
const button = (w: VueWrapper, id: 'prev' | 'next') =>
  w.get(`[data-testid="audience-members-${id}"]`).element as HTMLButtonElement
async function click(w: VueWrapper, id: 'prev' | 'next') {
  await w.get(`[data-testid="audience-members-${id}"]`).trigger('click')
  await flushPromises()
}

describe('AudienceMemberList', () => {
  describe('başlık ve not', () => {
    it('"Segmente uyan kişiler" başlığı ve adlandırılmış bir bölge gösterir', async () => {
      serve(3)
      const w = mountList()
      await flushPromises()

      expect(w.get('h2').text()).toBe('Segmente uyan kişiler')
      const region = w.get('[role="region"]')
      expect(region.attributes('aria-labelledby')).toBe('audience-members-title')
      expect(w.get('#audience-members-title').text()).toBe('Segmente uyan kişiler')
    })

    it('demo verisi olduğunu ve kesin alıcıların onayda yeniden belirlendiğini söyler', async () => {
      serve(3)
      const w = mountList()
      await flushPromises()

      const note = w.get('[data-testid="audience-members-note"]').text()
      expect(note).toContain('demo verisidir')
      expect(note).toContain('onaylanırken yeniden belirlenir')
      expect(note).toContain('İzni olmayan')
    })

    it('hata ve boş durumda da not görünür', async () => {
      fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))
      const w = mountList()
      await flushPromises()

      expect(w.find('[data-testid="audience-members-note"]').exists()).toBe(true)
    })
  })

  describe('yükleme', () => {
    it('ilk sayfayı 10 kişilik ister', async () => {
      serve(3)
      mountList()
      await flushPromises()

      expect(urls()).toEqual([`/api/campaigns/${ID}/audience?page=1&limit=10`])
    })

    it('yanıt gelene kadar iskelet gösterir, sonra satırları', async () => {
      const pending = deferred<Response>()
      fetchMock.mockReturnValue(pending.promise)
      const w = mountList()
      await flushPromises()

      const loading = w.get('[data-testid="audience-members-loading"]')
      expect(loading.attributes('role')).toBe('status')
      expect(loading.text()).toContain('Kişi listesi yükleniyor')
      expect(rows(w)).toHaveLength(0)
      expect(w.find('[data-testid="audience-members-pager"]').exists()).toBe(false)

      pending.resolve(jsonResponse(200, audiencePageBody({ items: members(3), total: 3 })))
      await flushPromises()

      expect(w.find('[data-testid="audience-members-loading"]').exists()).toBe(false)
      expect(rows(w)).toHaveLength(3)
    })
  })

  describe('satırlar', () => {
    it('ad, şehir, e-posta, telefon ve iki kanal uygunluğunu gösterir', async () => {
      const body = audiencePageBody({ items: [memberBody()] })
      fetchMock.mockResolvedValue(jsonResponse(200, body))
      const w = mountList()
      await flushPromises()

      const row = w.get('[data-testid="member-row"]')
      expect(row.get('[data-testid="member-name"]').text()).toContain('Ayşe')
      expect(row.get('[data-testid="member-city"]').text()).toContain('İstanbul')
      expect(row.get('[data-testid="member-email"]').text()).toContain('ayse@example.test')
      expect(row.get('[data-testid="member-phone"]').text()).toContain('+905550000001')
      expect(row.get('[data-testid="member-sms"] [data-status]').text()).toBe('Uygun')
      expect(row.get('[data-testid="member-email-status"] [data-status]').text()).toBe('Uygun')
    })

    it('tablo başlıkları altı sütunu adlandırır', async () => {
      serve(1)
      const w = mountList()
      await flushPromises()

      const heads = w.findAll('th').map((th) => th.text())
      expect(heads).toEqual([
        'Ad',
        'Şehir',
        'E-posta',
        'Telefon',
        'SMS uygunluğu',
        'E-posta uygunluğu',
      ])
    })

    it('eksik ad, e-posta ve telefonu açıkça "yok" olarak yazar', async () => {
      const person = memberBody({ first_name: null, email: null, phone: null })
      fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody({ items: [person] })))
      const w = mountList()
      await flushPromises()

      const missing = (testId: string) =>
        w.get(`[data-testid="${testId}"] [data-missing="true"]`).text()
      expect(missing('member-name')).toBe('Ad yok')
      expect(missing('member-email')).toBe('E-posta yok')
      expect(missing('member-phone')).toBe('Telefon yok')
    })

    it('boş metni de eksik sayar (boş hücre bırakmaz)', async () => {
      const person = memberBody({ first_name: '', email: '', phone: '' })
      fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody({ items: [person] })))
      const w = mountList()
      await flushPromises()

      expect(w.findAll('[data-missing="true"]')).toHaveLength(3)
    })

    it('dolu bilgide "yok" yazmaz', async () => {
      serve(1)
      const w = mountList()
      await flushPromises()

      expect(w.findAll('[data-missing="true"]')).toHaveLength(0)
    })

    it('izni olmayan kişiyi listeler ve ilgili kanal için uygun olmadığını gösterir', async () => {
      const person = memberBody({
        first_name: 'Deniz',
        sms_status: 'no_consent',
        email_status: 'no_address',
      })
      fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody({ items: [person] })))
      const w = mountList()
      await flushPromises()

      expect(rows(w)).toHaveLength(1)
      const sms = w.get('[data-testid="member-sms"] [data-status]')
      const mail = w.get('[data-testid="member-email-status"] [data-status]')
      expect(sms.attributes('data-status')).toBe('no_consent')
      expect(sms.text()).toBe('İzin yok')
      expect(mail.attributes('data-status')).toBe('no_address')
      expect(mail.text()).toBe('Adres yok')
    })

    it('adı HTML olarak yorumlamaz', async () => {
      const person = memberBody({ first_name: '<b>kalın</b><img src=x onerror=alert(1)>' })
      fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody({ items: [person] })))
      const w = mountList()
      await flushPromises()

      const row = w.get('[data-testid="member-row"]')
      expect(row.find('b').exists()).toBe(false)
      expect(row.find('img').exists()).toBe(false)
      expect(row.get('[data-testid="member-name"]').text()).toContain('<b>kalın</b>')
    })
  })

  describe('sayfalama', () => {
    it('ilk sayfada aralığı ve toplamı gösterir; Önceki kapalı, Sonraki açıktır', async () => {
      serve(25)
      const w = mountList()
      await flushPromises()

      expect(rows(w)).toHaveLength(10)
      expect(pagerText(w)).toBe('1–10 / 25 kişi · Sayfa 1 / 3')
      expect(button(w, 'prev').disabled).toBe(true)
      expect(button(w, 'next').disabled).toBe(false)
    })

    it('Sonraki bir sonraki sayfayı ister ve satırları o sayfayla değiştirir', async () => {
      serve(25)
      const w = mountList()
      await flushPromises()

      await click(w, 'next')

      expect(urls()).toEqual([
        `/api/campaigns/${ID}/audience?page=1&limit=10`,
        `/api/campaigns/${ID}/audience?page=2&limit=10`,
      ])
      expect(names(w)).toEqual(Array.from({ length: 10 }, (_, i) => `Kişi ${11 + i}`))
      expect(pagerText(w)).toBe('11–20 / 25 kişi · Sayfa 2 / 3')
      expect(button(w, 'prev').disabled).toBe(false)
      expect(button(w, 'next').disabled).toBe(false)
    })

    it('son sayfa kısa olabilir; Sonraki kapanır, Önceki önceki sayfaya döner', async () => {
      serve(25)
      const w = mountList()
      await flushPromises()
      await click(w, 'next')
      await click(w, 'next')

      expect(rows(w)).toHaveLength(5)
      expect(pagerText(w)).toBe('21–25 / 25 kişi · Sayfa 3 / 3')
      expect(button(w, 'next').disabled).toBe(true)

      await click(w, 'prev')

      expect(urls().slice(-1)).toEqual([`/api/campaigns/${ID}/audience?page=2&limit=10`])
      expect(pagerText(w)).toBe('11–20 / 25 kişi · Sayfa 2 / 3')
    })

    it('tam bir sayfa kadar kişi varsa tek sayfa olur ve iki düğme de kapalıdır', async () => {
      serve(10)
      const w = mountList()
      await flushPromises()

      expect(pagerText(w)).toBe('1–10 / 10 kişi · Sayfa 1 / 1')
      expect(button(w, 'prev').disabled).toBe(true)
      expect(button(w, 'next').disabled).toBe(true)
    })

    it('yeni sayfa yüklenirken eski satırlar görünür kalır, düğmeler kapanır ve tekrar tıklama istek yapmaz', async () => {
      serve(25)
      const w = mountList()
      await flushPromises()
      const pending = deferred<Response>()
      fetchMock.mockReturnValue(pending.promise)

      await w.get('[data-testid="audience-members-next"]').trigger('click')
      await flushPromises()

      expect(names(w)[0]).toBe('Kişi 1') // eski sayfa hâlâ görünür
      expect(w.get('[data-testid="audience-members-page"]').attributes('aria-busy')).toBe('true')
      expect(button(w, 'next').disabled).toBe(true)
      expect(button(w, 'prev').disabled).toBe(true)
      await w.get('[data-testid="audience-members-next"]').trigger('click')
      expect(fetchMock).toHaveBeenCalledTimes(2) // ilk sayfa + tek ikinci sayfa isteği

      pending.resolve(
        jsonResponse(200, audiencePageBody({ page: 2, total: 25, items: members(10, 11) })),
      )
      await flushPromises()

      expect(names(w)[0]).toBe('Kişi 11')
      expect(w.get('[data-testid="audience-members-page"]').attributes('aria-busy')).toBe('false')
    })

    it('liste kısalıp istenen sayfa boş kalırsa son sayfaya geçer', async () => {
      serve(25)
      const w = mountList()
      await flushPromises()
      await click(w, 'next') // sayfa 2
      // Bu arada liste 12 kişiye indi: sayfa 3 artık yok.
      fetchMock.mockImplementation((input) => {
        const page = Number(new URL(String(input), 'http://x').searchParams.get('page'))
        const items = page === 1 ? members(10, 1) : page === 2 ? members(2, 11) : []
        return Promise.resolve(jsonResponse(200, audiencePageBody({ page, total: 12, items })))
      })

      await click(w, 'next') // sayfa 3 boş döner → sayfa 2'ye düşer

      expect(urls().slice(-2)).toEqual([
        `/api/campaigns/${ID}/audience?page=3&limit=10`,
        `/api/campaigns/${ID}/audience?page=2&limit=10`,
      ])
      expect(pagerText(w)).toBe('11–12 / 12 kişi · Sayfa 2 / 2')
      expect(w.find('[data-testid="audience-members-empty"]').exists()).toBe(false)
    })
  })

  describe('boş sonuç', () => {
    it('eşleşen kişi yoksa açıklama gösterir; tablo ve sayfalama çıkmaz', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody({ items: [], total: 0 })))
      const w = mountList()
      await flushPromises()

      const empty = w.get('[data-testid="audience-members-empty"]')
      expect(empty.text()).toContain('Bu tanımla eşleşen kişi yok')
      expect(w.find('table').exists()).toBe(false)
      expect(w.find('[data-testid="audience-members-pager"]').exists()).toBe(false)
      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
    })
  })

  describe('hata', () => {
    it('yeniden denenebilir hatada başlık ve "Yeniden dene" gösterir; deneyince listeyi yükler', async () => {
      fetchMock.mockResolvedValueOnce(errorResponse(500, 'INTERNAL'))
      const w = mountList()
      await flushPromises()

      const panel = w.get('[data-testid="error-panel"]')
      expect(panel.text()).toContain('Kişi listesi yüklenemedi.')
      expect(panel.text()).toContain('Beklenmeyen bir hata oluştu.')
      expect(rows(w)).toHaveLength(0)

      serve(3)
      await w.get('[data-testid="error-retry"]').trigger('click')
      await flushPromises()

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
      expect(rows(w)).toHaveLength(3)
      expect(urls()).toEqual([
        `/api/campaigns/${ID}/audience?page=1&limit=10`,
        `/api/campaigns/${ID}/audience?page=1&limit=10`,
      ])
    })

    it('yeniden denenemeyen hatada (kampanya yok) "Yeniden dene" sunmaz', async () => {
      fetchMock.mockResolvedValue(errorResponse(404, 'CAMPAIGN_NOT_FOUND'))
      const w = mountList()
      await flushPromises()

      expect(w.get('[data-testid="error-panel"]').text()).toContain('Kişi listesi yüklenemedi.')
      expect(w.find('[data-testid="error-retry"]').exists()).toBe(false)
    })

    it('ağ hatasını açıklar', async () => {
      fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
      const w = mountList()
      await flushPromises()

      expect(w.get('[data-testid="error-panel"]').text()).toContain('Sunucuya ulaşılamadı.')
      expect(w.find('[data-testid="error-retry"]').exists()).toBe(true)
    })

    it('geçersiz yanıtı beklenmeyen yanıt olarak gösterir (yarım veri çizmez)', async () => {
      fetchMock.mockResolvedValue(jsonResponse(200, { page: 1, limit: 10, total: 5, items: 'x' }))
      const w = mountList()
      await flushPromises()

      expect(w.get('[data-testid="error-panel"]').text()).toContain(
        'Sunucudan beklenmeyen bir yanıt alındı.',
      )
      expect(rows(w)).toHaveLength(0)
    })

    it('sonraki sayfada hata olursa yeniden deneme o sayfayı ister', async () => {
      serve(25)
      const w = mountList()
      await flushPromises()
      fetchMock.mockResolvedValueOnce(errorResponse(500, 'INTERNAL'))

      await click(w, 'next')

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(true)
      serve(25)
      await w.get('[data-testid="error-retry"]').trigger('click')
      await flushPromises()

      expect(urls().slice(-2)).toEqual([
        `/api/campaigns/${ID}/audience?page=2&limit=10`,
        `/api/campaigns/${ID}/audience?page=2&limit=10`,
      ])
      expect(pagerText(w)).toBe('11–20 / 25 kişi · Sayfa 2 / 3')
    })
  })

  describe('yaşam döngüsü', () => {
    it('kampanya değişince eski satırları bırakır, yeni kampanyayı ister ve eskinin geç yanıtını yok sayar', async () => {
      const slow = deferred<Response>()
      fetchMock.mockImplementation((input) => {
        if (String(input).includes('/eski/')) return slow.promise
        const items = [memberBody({ first_name: 'Yeni' })]
        return Promise.resolve(jsonResponse(200, audiencePageBody({ items })))
      })
      const w = mountList('eski')
      await flushPromises()

      await w.setProps({ campaignId: 'yeni' })
      await flushPromises()
      slow.resolve(
        jsonResponse(200, audiencePageBody({ items: [memberBody({ first_name: 'Eski' })] })),
      )
      await flushPromises()

      expect(urls()).toEqual([
        '/api/campaigns/eski/audience?page=1&limit=10',
        '/api/campaigns/yeni/audience?page=1&limit=10',
      ])
      expect(names(w)).toEqual(['Yeni'])
    })

    it("kampanya değişince sayfa 1'e döner", async () => {
      serve(25)
      const w = mountList('bir')
      await flushPromises()
      await click(w, 'next')

      await w.setProps({ campaignId: 'iki' })
      await flushPromises()

      expect(urls().slice(-1)).toEqual(['/api/campaigns/iki/audience?page=1&limit=10'])
      expect(pagerText(w)).toBe('1–10 / 25 kişi · Sayfa 1 / 3')
    })

    it('bileşen kaldırılınca süren isteği iptal eder', async () => {
      fetchMock.mockReturnValue(deferred<Response>().promise)
      const w = mountList()
      await flushPromises()

      w.unmount()

      expect(fetchMock.mock.calls[0]![1]?.signal?.aborted).toBe(true)
    })

    it('iptal edilen isteği hata olarak göstermez', async () => {
      fetchMock.mockRejectedValue(new DOMException('iptal', 'AbortError'))
      const w = mountList()
      await flushPromises()

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
    })
  })
})
