import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import CampaignPreviewView from '../CampaignPreviewView.vue'
import {
  audiencePageBody,
  campaignBody,
  deferred,
  errorResponse,
  jsonResponse,
  memberBody,
} from '@/__tests__/helpers'
import { campaignListVersion } from '@/campaign/recent'
import { notify } from '@/notify/notify'

const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

// Kişi listesi kendi isteğini yapar; bu dosyadaki testler tüm `fetch` çağrılarına aynı yanıtı
// verdiği için liste burada sahte bileşenle değiştirilir. Listenin kendisi
// `AudienceMemberList.spec.ts`'te, ekranla birlikte çalışması aşağıdaki "kişi listesi"
// bölümünde sınanır.
function mountView() {
  return mount(CampaignPreviewView, {
    global: { stubs: { RouterLink: RouterLinkStub, AudienceMemberList: true } },
  })
}

async function submit(w: VueWrapper, brief: string) {
  await w.get('textarea').setValue(brief)
  await w.get('form').trigger('submit')
  await flushPromises()
}

const textarea = (w: VueWrapper) => w.get('textarea').element as HTMLTextAreaElement
const requestedBriefs = () =>
  fetchMock.mock.calls.map(([, init]) => JSON.parse(String(init?.body)).brief as string)

const stepStates = (w: VueWrapper) =>
  w.findAll('[data-testid="campaign-steps"] li').map((li) => li.attributes('data-status'))

describe('CampaignPreviewView', () => {
  describe('üst bölüm ve adım göstergesi', () => {
    it('başlık ve kısa açıklama görünür; kampanya yokken rozet yok, kitle adımı sürer', () => {
      const w = mountView()

      expect(w.get('h1').text()).toBe('Kampanya önizleme')
      expect(w.get('[data-testid="page-description"]').text()).toContain('kitleyi')
      expect(w.find('[data-testid="campaign-state"]').exists()).toBe(false)
      expect(stepStates(w)).toEqual(['current', 'upcoming', 'upcoming', 'upcoming'])
    })

    it('kampanya kaydedilince Taslak rozeti çıkar ve içerik adımı sürer', async () => {
      fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))
      const w = mountView()

      await submit(w, 'İstanbul sepet')

      expect(w.get('[data-testid="campaign-state"]').text()).toBe('Taslak')
      expect(stepStates(w)).toEqual(['done', 'current', 'upcoming', 'upcoming'])
    })

    it('hata olursa kampanya oluşmadığı için rozet çıkmaz, kitle adımı sürer', async () => {
      fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))
      const w = mountView()

      await submit(w, 'İstanbul sepet')

      expect(w.find('[data-testid="campaign-state"]').exists()).toBe(false)
      expect(stepStates(w)).toEqual(['current', 'upcoming', 'upcoming', 'upcoming'])
    })
  })

  describe('başarılı önizleme', () => {
    it('kırpılmış brief gönderir, üç kitle sayısını ve anlaşılan koşulları gösterir', async () => {
      fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))
      const w = mountView()

      await submit(w, '  İstanbul sepet  ')

      expect(requestedBriefs()).toEqual(['İstanbul sepet'])
      expect(w.get('[data-testid="total"]').text()).toBe('190')
      expect(w.get('[data-testid="sms"]').text()).toBe('140')
      expect(w.get('[data-testid="email"]').text()).toBe('150')
      const lines = w.findAll('[aria-labelledby="segment-title"] li').map((li) => li.text())
      expect(lines).toEqual([
        'Şehir: İstanbul',
        'Son 30 günde sepete ürün eklemiş',
        'Son 30 günde satın alma yapmamış',
      ])
      expect(w.text()).toContain('498882b6-2221-4589-a99f-67ceb68c59c2')
      expect(w.find('[role="alert"]').exists()).toBe(false)
    })

    it('kaydedilen kampanyanın içerik ekranına bağlantı verir', async () => {
      fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))
      const w = mountView()
      expect(w.findComponent(RouterLinkStub).exists()).toBe(false) // önizleme yokken yok

      await submit(w, 'İstanbul sepet')

      const link = w.getComponent(RouterLinkStub)
      expect(link.props('to')).toEqual({
        name: 'campaign',
        params: { id: '498882b6-2221-4589-a99f-67ceb68c59c2' },
      })
      expect(link.text()).toBe('İleri: İçeriği hazırla')
    })

    it('"İleri" kitle hesaplanana kadar kapalıdır ve nedenini söyler; "Geri" yoktur (ilk aşama)', async () => {
      const w = mountView()
      const next = w.get('[data-testid="nav-next-disabled"]')

      expect(next.attributes('disabled')).toBeDefined()
      expect(next.text()).toBe('İleri: İçeriği hazırla')
      expect(w.get('#step-nav-next-hint').text()).toBe('Önce kitleyi hesaplayın.')
      expect(w.text()).not.toContain('Geri')

      fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))
      await submit(w, 'İstanbul sepet')

      expect(w.find('[data-testid="nav-next-disabled"]').exists()).toBe(false)
      expect(w.get('[data-testid="nav-next"]').text()).toBe('İleri: İçeriği hazırla')
    })

    it('hatada "İleri" kapalı kalır (kampanya oluşmadı)', async () => {
      fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))
      const w = mountView()
      await submit(w, 'İstanbul sepet')
      expect(w.find('[data-testid="nav-next"]').exists()).toBe(false)
      expect(w.find('[data-testid="nav-next-disabled"]').exists()).toBe(true)
    })

    it('sıfır eşleşmeyi hata değil önizleme olarak gösterir', async () => {
      fetchMock.mockResolvedValue(
        jsonResponse(201, campaignBody({ audience: { total: 0, sms: 0, email: 0 } })),
      )
      const w = mountView()

      await submit(w, 'Kars')

      expect(w.get('[data-testid="total"]').text()).toBe('0')
      expect(w.text()).toContain('hiç kullanıcı eşleşmedi')
      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
    })

    it('istek sürerken durum gösterir ve düğmeyi devre dışı bırakır', async () => {
      const pending = deferred<Response>()
      fetchMock.mockReturnValue(pending.promise)
      const w = mountView()

      await submit(w, 'Bursa')

      expect(w.get('[role="status"]').text()).toContain('Kitle hesaplanıyor')
      expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
      expect(w.find('[data-testid="total"]').exists()).toBe(false)

      pending.resolve(jsonResponse(201, campaignBody()))
      await flushPromises()

      expect(w.get('[data-testid="total"]').text()).toBe('190')
      expect(w.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    })
  })

  describe('boş brief', () => {
    it('istek göndermeden hata gösterir', async () => {
      const w = mountView()

      await submit(w, '   ')

      expect(fetchMock).not.toHaveBeenCalled()
      expect(w.get('[role="alert"]').text()).toBe('Brief boş olamaz.')
    })
  })

  describe('desteklenmeyen segment (SEGMENT_INVALID)', () => {
    const reasons = [
      { code: 'UNSUPPORTED_FIELD', field: 'favorite_color' },
      { code: 'UNSUPPORTED_PART', text: 'ürün kategorisi' },
    ]

    it('sebepleri Türkçe gösterir, kitle sayısı göstermez, brief düzenlenebilir kalır', async () => {
      fetchMock.mockResolvedValue(errorResponse(422, 'SEGMENT_INVALID', reasons))
      const w = mountView()

      await submit(w, 'Mavi sevenlere indirim, ürün kategorisine göre')

      const panel = w.get('[data-testid="error-panel"]')
      expect(panel.text()).toContain("Brief'ten geçerli bir kitle tanımı çıkarılamadı.")
      const items = panel.findAll('li').map((li) => li.text())
      expect(items[0]).toContain('"favorite_color" alanı desteklenmiyor')
      expect(items[1]).toBe('Brief\'in şu kısmı ifade edilemedi: "ürün kategorisi".')
      expect(panel.text()).toContain("Brief'i düzenleyip yeniden deneyebilirsiniz.")

      expect(w.find('[data-testid="total"]').exists()).toBe(false)
      expect(w.find('#segment-title').exists()).toBe(false)
      expect(textarea(w).value).toBe('Mavi sevenlere indirim, ürün kategorisine göre')
      expect(textarea(w).disabled).toBe(false)
      expect(w.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    })

    it('desteklenmeyen segmentte yeniden dene düğmesi göstermez, brief düzenlenip gönderilebilir', async () => {
      fetchMock.mockResolvedValueOnce(errorResponse(422, 'SEGMENT_INVALID', reasons))
      const w = mountView()
      await submit(w, 'Mavi sevenler')
      expect(w.find('[data-testid="error-retry"]').exists()).toBe(false)

      fetchMock.mockResolvedValueOnce(jsonResponse(201, campaignBody()))
      await submit(w, 'İstanbul sepet')

      expect(requestedBriefs()).toEqual(['Mavi sevenler', 'İstanbul sepet'])
      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
      expect(w.get('[data-testid="total"]').text()).toBe('190')
    })

    it('model kaynaklı metni HTML olarak yorumlamaz', async () => {
      fetchMock.mockResolvedValue(
        errorResponse(422, 'SEGMENT_INVALID', [
          { code: 'UNSUPPORTED_PART', text: '<img src=x onerror=alert(1)>' },
        ]),
      )
      const w = mountView()

      await submit(w, 'x')

      expect(w.find('[data-testid="error-panel"] img').exists()).toBe(false)
      expect(w.get('[data-testid="error-panel"]').text()).toContain('<img src=x onerror=alert(1)>')
    })
  })

  describe('yeniden denenebilir hatalar', () => {
    it.each([
      ['LLM_TIMEOUT', 504, 'zaman aşımına uğradı'],
      ['LLM_RATE_LIMITED', 503, 'şu an yoğun'],
      ['INTERNAL', 500, 'Beklenmeyen bir hata'],
    ])(
      '%s: mesaj gösterir ve yeniden dene ile aynı brief gönderilir',
      async (code, status, text) => {
        fetchMock.mockResolvedValueOnce(errorResponse(status, code))
        const w = mountView()
        await submit(w, 'Bursa sepet')

        expect(w.get('[data-testid="error-panel"]').text()).toContain(text)
        expect(w.find('[data-testid="total"]').exists()).toBe(false)
        expect(textarea(w).value).toBe('Bursa sepet')

        fetchMock.mockResolvedValueOnce(jsonResponse(201, campaignBody()))
        await w.get('[data-testid="error-retry"]').trigger('click')
        await flushPromises()

        expect(requestedBriefs()).toEqual(['Bursa sepet', 'Bursa sepet'])
        expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
        expect(w.get('[data-testid="total"]').text()).toBe('190')
      },
    )

    it('yeniden dene, kutuda sonradan değiştirilen metni değil son gönderileni kullanır', async () => {
      fetchMock.mockResolvedValueOnce(errorResponse(504, 'LLM_TIMEOUT'))
      const w = mountView()
      await submit(w, 'Bursa')
      await w.get('textarea').setValue('Başka bir şey yazıldı')

      fetchMock.mockResolvedValueOnce(jsonResponse(201, campaignBody()))
      await w.get('[data-testid="error-retry"]').trigger('click')
      await flushPromises()

      expect(requestedBriefs()).toEqual(['Bursa', 'Bursa'])
    })

    it('ağ hatasında sunucuya ulaşılamadığını söyler', async () => {
      fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
      const w = mountView()

      await submit(w, 'Bursa')

      expect(w.get('[data-testid="error-panel"]').text()).toContain('Sunucuya ulaşılamadı')
      expect(w.find('[data-testid="error-retry"]').exists()).toBe(true)
    })

    it('JSON olmayan 502 yanıtını beklenmeyen yanıt olarak gösterir', async () => {
      fetchMock.mockResolvedValueOnce(new Response('<html>Bad Gateway</html>', { status: 502 }))
      const w = mountView()

      await submit(w, 'Bursa')

      expect(w.get('[data-testid="error-panel"]').text()).toContain('beklenmeyen bir yanıt')
      expect(w.find('[data-testid="error-retry"]').exists()).toBe(true)
    })

    it('bozuk başarılı yanıtta kitle sayısı göstermez', async () => {
      fetchMock.mockResolvedValueOnce(jsonResponse(201, { id: 'x' }))
      const w = mountView()

      await submit(w, 'Bursa')

      expect(w.find('[data-testid="total"]').exists()).toBe(false)
      expect(w.find('[data-testid="error-panel"]').exists()).toBe(true)
    })
  })

  describe('yeniden gönderme', () => {
    it('yeni istek başlarken önceki hatayı ve sonucu temizler', async () => {
      fetchMock.mockResolvedValueOnce(errorResponse(504, 'LLM_TIMEOUT'))
      const w = mountView()
      await submit(w, 'Bursa')
      expect(w.find('[data-testid="error-panel"]').exists()).toBe(true)

      const pending = deferred<Response>()
      fetchMock.mockReturnValueOnce(pending.promise)
      await submit(w, 'Bursa sepet')

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
      expect(w.find('[role="status"]').exists()).toBe(true)
      pending.resolve(jsonResponse(201, campaignBody()))
    })

    it('geç gelen eski yanıt yeni sonucu ezmez ve eski istek iptal edilir', async () => {
      const first = deferred<Response>()
      const second = deferred<Response>()
      fetchMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
      const w = mountView()

      await submit(w, 'Birinci')
      await submit(w, 'İkinci')

      second.resolve(
        jsonResponse(201, campaignBody({ audience: { total: 22, sms: 11, email: 5 } })),
      )
      await flushPromises()
      first.resolve(
        jsonResponse(201, campaignBody({ audience: { total: 999, sms: 999, email: 999 } })),
      )
      await flushPromises()

      expect(w.get('[data-testid="total"]').text()).toBe('22')
      expect(fetchMock.mock.calls[0]![1]?.signal?.aborted).toBe(true)
      expect(fetchMock.mock.calls[1]![1]?.signal?.aborted).toBe(false)
    })

    it('eski isteğin hatası yeni sonucu ezmez', async () => {
      const first = deferred<Response>()
      const second = deferred<Response>()
      fetchMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
      const w = mountView()

      await submit(w, 'Birinci')
      await submit(w, 'İkinci')
      second.resolve(jsonResponse(201, campaignBody()))
      await flushPromises()
      first.reject(new DOMException('Aborted', 'AbortError'))
      await flushPromises()

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
      expect(w.get('[data-testid="total"]').text()).toBe('190')
    })
  })

  it('bileşen kaldırılınca süren isteği iptal eder', async () => {
    fetchMock.mockReturnValue(deferred<Response>().promise)
    const w = mountView()
    await submit(w, 'Bursa')

    w.unmount()

    expect(fetchMock.mock.calls[0]![1]?.signal?.aborted).toBe(true)
  })
})

// Kitle hesabının sonucu bir bildirimdir: kaydedilince başarı, brief düzeltilebilirse uyarı, aksi hata.
describe('eylem bildirimleri', () => {
  const spies = () => ({
    success: vi.spyOn(notify, 'success').mockReturnValue('s'),
    warning: vi.spyOn(notify, 'warning').mockReturnValue('w'),
    error: vi.spyOn(notify, 'error').mockReturnValue('e'),
  })
  afterEach(() => vi.restoreAllMocks())

  it('kampanya kaydedilince başarı bildirimi verir ve eşleşen kişi sayısını söyler', async () => {
    const n = spies()
    fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))
    const w = mountView()

    await submit(w, 'İstanbul sepet')

    expect(n.success).toHaveBeenCalledWith(
      'Taslak kampanya kaydedildi',
      'Kitle hesaplandı: 190 kişi eşleşti.',
    )
    expect(n.error).not.toHaveBeenCalled()
    expect(n.warning).not.toHaveBeenCalled()
  })

  it('brief desteklenmiyorsa (düzeltilebilir) uyarı verir, hata değil', async () => {
    const n = spies()
    fetchMock.mockResolvedValue(
      errorResponse(422, 'SEGMENT_INVALID', [{ code: 'UNSUPPORTED_FIELD', field: 'x' }]),
    )
    const w = mountView()

    await submit(w, 'garip brief')

    expect(n.warning).toHaveBeenCalledTimes(1)
    expect(n.warning.mock.calls[0]![0]).toBe('Kitle hesaplanamadı')
    expect(n.error).not.toHaveBeenCalled()
    expect(n.success).not.toHaveBeenCalled()
  })

  it('boş brief de uyarıdır', async () => {
    const n = spies()
    fetchMock.mockResolvedValue(errorResponse(422, 'BRIEF_EMPTY'))
    const w = mountView()

    await submit(w, 'x')

    expect(n.warning).toHaveBeenCalledTimes(1)
    expect(n.error).not.toHaveBeenCalled()
  })

  it('sunucu hatasında hata bildirimi verir', async () => {
    const n = spies()
    fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))
    const w = mountView()

    await submit(w, 'İstanbul sepet')

    expect(n.error).toHaveBeenCalledWith('Kitle hesaplanamadı', 'Beklenmeyen bir hata oluştu.')
    expect(n.success).not.toHaveBeenCalled()
  })

  it('iptal edilen istek bildirim üretmez', async () => {
    const n = spies()
    fetchMock.mockRejectedValue(new DOMException('iptal', 'AbortError'))
    const w = mountView()

    await submit(w, 'İstanbul sepet')

    expect(n.error).not.toHaveBeenCalled()
    expect(n.warning).not.toHaveBeenCalled()
  })
})

// "Son kampanyalar" listesi kenar çubuğundadır (AppSidebar). Bu ekran listeyi kendisi çizmez;
// kampanya oluşunca yalnızca haber verir ve liste kendini yeniler.
describe('kenar çubuğundaki son kampanyalar listesine haber verme', () => {
  it('ekranda liste yoktur (liste kenar çubuğundadır)', () => {
    const w = mountView()
    expect(w.find('[data-testid="recent-campaigns"]').exists()).toBe(false)
  })

  it('kampanya oluşunca listeye haber verir', async () => {
    fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))
    const w = mountView()
    const before = campaignListVersion.value

    await submit(w, 'İstanbul sepet')

    expect(campaignListVersion.value).toBe(before + 1)
  })

  it('hata olunca kampanya oluşmadığı için haber vermez', async () => {
    fetchMock.mockResolvedValue(errorResponse(422, 'BRIEF_EMPTY'))
    const w = mountView()
    const before = campaignListVersion.value

    await submit(w, 'x')

    expect(campaignListVersion.value).toBe(before)
  })
})

// ADR-031: kitle sayılarının altındaki "Segmente uyan kişiler" listesi bu ekranla birlikte çalışır.
// Burada gerçek bileşen kullanılır; `fetch` adrese göre yönlendirilir.
describe('segmente uyan kişiler listesi', () => {
  const CAMPAIGN_ID = '498882b6-2221-4589-a99f-67ceb68c59c2'

  function mountWithList() {
    return mount(CampaignPreviewView, { global: { stubs: { RouterLink: RouterLinkStub } } })
  }

  /** POST /api/campaigns kampanya, GET .../audience liste döndürür. */
  function routeFetch(list: () => Response, campaign = campaignBody()) {
    fetchMock.mockImplementation((input) =>
      Promise.resolve(String(input).includes('/audience') ? list() : jsonResponse(201, campaign)),
    )
  }
  const audienceCalls = () =>
    fetchMock.mock.calls.map(([url]) => String(url)).filter((url) => url.includes('/audience'))

  it('kampanya oluşunca listeyi o kampanyanın kimliğiyle ister; sayıların ve kural özetinin altında gösterir', async () => {
    routeFetch(() => jsonResponse(200, audiencePageBody()))
    const w = mountWithList()

    await submit(w, 'İstanbul sepet')

    expect(audienceCalls()).toEqual([`/api/campaigns/${CAMPAIGN_ID}/audience?page=1&limit=10`])
    expect(w.get('[data-testid="audience-members"] h2').text()).toBe('Segmente uyan kişiler')
    expect(w.findAll('[data-testid="member-row"]')).toHaveLength(1)
    const html = w.html()
    const at = (id: string) => html.indexOf(`id="${id}"`)
    expect(at('audience-title')).toBeGreaterThan(-1)
    expect(at('segment-title')).toBeGreaterThan(at('audience-title'))
    expect(at('audience-members-title')).toBeGreaterThan(at('segment-title'))
  })

  it('kampanya oluşmadan liste yoktur ve istenmez', async () => {
    const w = mountWithList()

    expect(w.find('[data-testid="audience-members"]').exists()).toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('kampanya oluşamazsa (hata) liste istenmez ve gösterilmez', async () => {
    fetchMock.mockResolvedValue(errorResponse(422, 'BRIEF_EMPTY'))
    const w = mountWithList()

    await submit(w, 'x')

    expect(audienceCalls()).toEqual([])
    expect(w.find('[data-testid="audience-members"]').exists()).toBe(false)
  })

  it('liste yüklenemese de kitle sayıları ve kural özeti görünür; listede hata ve yeniden dene çıkar', async () => {
    routeFetch(() => errorResponse(500, 'INTERNAL'))
    const w = mountWithList()

    await submit(w, 'İstanbul sepet')

    expect(w.get('[data-testid="total"]').text()).toBe('190')
    expect(w.find('[aria-labelledby="segment-title"]').exists()).toBe(true)
    const list = w.get('[data-testid="audience-members"]')
    expect(list.get('[data-testid="error-panel"]').text()).toContain('Kişi listesi yüklenemedi')
    expect(list.find('[data-testid="error-retry"]').exists()).toBe(true)
    // Kampanya hatası paneli (sayfa düzeyi) çıkmaz: yalnızca listenin kendi paneli vardır.
    expect(w.findAll('[data-testid="error-panel"]')).toHaveLength(1)
  })

  it('yeni brief gönderilince eski kampanyanın satırları kalkar, yeni kampanyanın listesi istenir', async () => {
    const second = 'aaaaaaaa-1111-4222-8333-bbbbbbbbbbbb'
    const first = deferred<Response>()
    let created = 0 // POST /api/campaigns çağrıları: ilki birinci, ikincisi ikinci kampanya
    fetchMock.mockImplementation((input) => {
      const url = String(input)
      if (url.includes(`/${CAMPAIGN_ID}/audience`)) return first.promise
      if (url.includes('/audience')) {
        const items = [memberBody({ first_name: 'Yeni kampanya' })]
        return Promise.resolve(jsonResponse(200, audiencePageBody({ items })))
      }
      created += 1
      return Promise.resolve(jsonResponse(201, campaignBody(created === 1 ? {} : { id: second })))
    })
    const w = mountWithList()

    await submit(w, 'birinci') // kampanya 1: listesi henüz gelmedi
    await submit(w, 'ikinci') // kampanya 2
    const stale = [memberBody({ first_name: 'Eski kampanya' })]
    first.resolve(jsonResponse(200, audiencePageBody({ items: stale })))
    await flushPromises()

    expect(audienceCalls()).toEqual([
      `/api/campaigns/${CAMPAIGN_ID}/audience?page=1&limit=10`,
      `/api/campaigns/${second}/audience?page=1&limit=10`,
    ])
    const names = w
      .findAll('[data-testid="member-name"]')
      .map((n) => n.get('span:last-child').text())
    expect(names).toEqual(['Yeni kampanya'])
  })
})
