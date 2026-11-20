import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import DeliveryDashboardView from '../DeliveryDashboardView.vue'
import {
  campaignBody,
  controlledSse,
  deferred,
  errorResponse,
  jsonResponse,
  sseEvent,
  type ControlledSse,
} from '@/__tests__/helpers'

const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'
const OTHER = '11111111-2222-4333-8444-555555555555'
const fetchMock = vi.fn<typeof fetch>()

interface Stream {
  path: string
  signal: AbortSignal | null | undefined
  sse: ControlledSse
}
let streams: Stream[] = []
type Handler = () => Response | Promise<Response>
let handlers: Record<string, Handler> = {}
const calls: string[] = []
let mounted: VueWrapper[] = []

const detailPath = (id: string) => `/api/campaigns/${id}`
const eventsPath = (id: string) => `/api/campaigns/${id}/events`
const callsTo = (path: string) => calls.filter((c) => c === path)

function approved(id = ID, extra: Record<string, unknown> = {}) {
  return jsonResponse(200, {
    ...campaignBody({ id, status: 'approved', brief: `Brief ${id.slice(0, 4)}` }),
    approved_content_version: 1,
    approved_at: '2026-11-19T10:00:00Z',
    ...extra,
  })
}

beforeEach(() => {
  streams = []
  handlers = {}
  calls.length = 0
  mounted = []
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    const path = String(input)
    calls.push(path)
    if (path.endsWith('/events')) {
      const handler = handlers[path]
      if (handler) return handler()
      const sse = controlledSse(init?.signal)
      streams.push({ path, signal: init?.signal, sse })
      return sse.response
    }
    const handler = handlers[path]
    if (!handler) throw new Error(`beklenmeyen istek: ${path}`)
    return handler()
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  mounted.forEach((w) => w.unmount())
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function stream(index: number): Stream {
  const found = streams[index]
  if (!found) throw new Error(`${index}. akış yok (toplam ${streams.length})`)
  return found
}

async function mountView(id = ID): Promise<VueWrapper> {
  const w = mount(DeliveryDashboardView, {
    props: { id },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
  mounted.push(w)
  await flushPromises()
  return w
}

const snap = (messages: Record<string, number>, extra: Record<string, unknown> = {}) =>
  sseEvent('snapshot', {
    status: 'sending',
    messages: { queued: 0, retrying: 0, sending: 0, sent: 0, failed: 0, skipped: 0, ...messages },
    failures: { exhausted: 0, rejected: 0 },
    ...extra,
  })

async function push(index: number, text: string) {
  stream(index).sse.push(text)
  await flushPromises()
}

const count = (w: VueWrapper, key: string) => w.get(`[data-testid="count-${key}"]`).text()

const stepStates = (w: VueWrapper) =>
  w.findAll('[data-testid="campaign-steps"] li').map((li) => li.attributes('data-status'))

describe('üst bölüm, adım göstergesi ve özet sayılar', () => {
  it('başlık, kısa açıklama ve durum rozeti üsttedir; gönderim adımı sürer', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    expect(w.get('h1').text()).toBe('Gönderim takibi')
    expect(w.get('[data-testid="page-description"]').text()).toContain('canlı')
    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Onaylandı, kuyrukta')
    expect(stepStates(w)).toEqual(['done', 'done', 'done', 'current'])
  })

  it('canlı görüntü tamamlandı bildirince dört adım da biter', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    await push(0, snap({ sent: 3 }, { status: 'completed' }))

    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Tamamlandı')
    expect(stepStates(w)).toEqual(['done', 'done', 'done', 'done'])
  })

  it('onaylanmamış kampanyada adım göstergesi çıkmaz, rozet Taslak der', async () => {
    handlers[detailPath(ID)] = () => jsonResponse(200, campaignBody())
    const w = await mountView()

    expect(w.find('[data-testid="campaign-steps"]').exists()).toBe(false)
    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Taslak')
  })

  it('canlı görüntüdeki sayılar üstte gönderildi, başarısız ve tamamlanmamış olarak görünür', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    await push(0, snap({ queued: 2, retrying: 1, sending: 1, sent: 10, failed: 3, skipped: 1 }))

    const summary = (key: string) => w.get(`[data-testid="summary-${key}"]`).text()
    expect(summary('sent')).toBe('10')
    expect(summary('failed')).toBe('3')
    expect(summary('unfinished')).toBe('4')
    expect(count(w, 'sent')).toBe('10') // ayrıntı aynı görüntüden gelir
  })
})

describe('kanal dağılımı ve toparlananlar (ekran)', () => {
  const zero = { queued: 0, retrying: 0, sending: 0, sent: 0, failed: 0, skipped: 0 }
  const live = {
    channels: {
      sms: { ...zero, sent: 4, failed: 1 },
      email: { ...zero, sent: 2 },
    },
    recovered: 3,
  }

  it('canlı görüntüde kanal kırılımını ve toparlanan sayısını gösterir', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    await push(
      0,
      snap({ sent: 6, failed: 1 }, { failures: { exhausted: 1, rejected: 0 }, ...live }),
    )

    expect(w.get('[data-testid="channel-sms-sent"]').text()).toBe('4')
    expect(w.get('[data-testid="channel-sms-failed"]').text()).toBe('1')
    expect(w.get('[data-testid="channel-email-sent"]').text()).toBe('2')
    expect(w.get('[data-testid="recovered-count"]').text()).toBe('3')
  })

  it('tamamlanınca özet, toparlananları gönderilen satırında söyler', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    await push(
      0,
      snap(
        { sent: 6, failed: 1 },
        { status: 'completed', failures: { exhausted: 1, rejected: 0 }, ...live },
      ),
    )

    expect(w.get('[data-testid="completion-lines"]').text()).toContain(
      '3 mesaj ilk denemede tamamlanamadı, yeniden denemeyle gönderildi',
    )
  })

  it('kanallar toplama eşit değilse görüntü geçersiz sayılır ve sayılar değişmez', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()
    await push(0, snap({ sent: 6, failed: 1 }, live))
    expect(w.get('[data-testid="summary-sent"]').text()).toBe('6')

    // SMS gönderilen 1 fazla: kanalların toplamı 7 ≠ 6.
    await push(
      0,
      snap(
        { sent: 6, failed: 1 },
        { channels: { sms: { ...zero, sent: 5, failed: 1 }, email: { ...zero, sent: 2 } } },
      ),
    )

    expect(w.get('[data-testid="channel-sms-sent"]').text()).toBe('4') // eski geçerli değer
  })
})

describe('Geri ve İleri', () => {
  const navLink = (w: VueWrapper, id: string) =>
    w.findAllComponents(RouterLinkStub).find((l) => l.attributes('data-testid') === id)

  it('"Geri" kampanyaya döner; "İleri" yeni kampanyaya gider ve gönderimin bitmesini beklemez', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    expect(navLink(w, 'nav-back')?.props('to')).toBe(`/campaigns/${ID}`)
    expect(navLink(w, 'nav-back')?.text()).toBe('Geri: Kampanyaya dön')
    expect(navLink(w, 'nav-next')?.props('to')).toBe('/')
    expect(navLink(w, 'nav-next')?.text()).toBe('İleri: Yeni kampanya')
    expect(w.find('[data-testid="nav-next-disabled"]').exists()).toBe(false)
  })

  it('gönderim sürerken kullanıcıya gönderimin arka planda devam ettiğini söyler', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    await push(0, snap({ sent: 1, queued: 2 }))

    expect(navLink(w, 'nav-next')?.props('to')).toBe('/') // gönderim sürerken de açık
    expect(w.get('#step-nav-next-hint').text()).toBe('Gönderim arka planda devam eder.')
    expect(navLink(w, 'nav-next')?.attributes('aria-describedby')).toBe('step-nav-next-hint')
  })

  it('gönderim tamamlanınca not kalkar, "İleri" yine açıktır', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    await push(0, snap({ sent: 3 }, { status: 'completed' }))

    expect(navLink(w, 'nav-next')?.props('to')).toBe('/')
    expect(w.find('#step-nav-next-hint').exists()).toBe(false)
  })

  it('kampanya henüz onaylanmamışken de yeni kampanya açılabilir', async () => {
    handlers[detailPath(ID)] = () => jsonResponse(200, campaignBody())
    const w = await mountView()
    expect(navLink(w, 'nav-next')?.props('to')).toBe('/')
  })

  it('kampanya yüklenemese de "Geri" kullanılabilir', async () => {
    handlers[detailPath(ID)] = () => errorResponse(404, 'CAMPAIGN_NOT_FOUND')
    const w = await mountView()
    expect(navLink(w, 'nav-back')?.props('to')).toBe(`/campaigns/${ID}`)
  })

  it('başka kampanyaya geçilince "Geri" yeni kampanyaya işaret eder', async () => {
    handlers[detailPath(ID)] = () => approved()
    handlers[detailPath(OTHER)] = () => approved(OTHER)
    const w = await mountView()
    await w.setProps({ id: OTHER })
    await flushPromises()
    expect(navLink(w, 'nav-back')?.props('to')).toBe(`/campaigns/${OTHER}`)
  })
})

describe('yükleme', () => {
  it('kampanya yüklenirken iskelet gösterir; yüklenince brief, durum ve onay zamanını gösterir', async () => {
    const detail = deferred<Response>()
    handlers[detailPath(ID)] = () => detail.promise
    const w = mount(DeliveryDashboardView, {
      props: { id: ID },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    mounted.push(w)
    await flushPromises()

    expect(w.get('[data-testid="loading"]').text()).toContain('Kampanya yükleniyor')
    expect(w.find('[data-testid="campaign-status"]').exists()).toBe(false)
    expect(callsTo(eventsPath(ID))).toHaveLength(0) // kampanya bilinmeden bağlanılmaz

    detail.resolve(approved())
    await flushPromises()

    expect(w.find('[data-testid="loading"]').exists()).toBe(false)
    expect(w.text()).toContain(`Brief ${ID.slice(0, 4)}`)
    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Onaylandı, kuyrukta')
    expect(w.text()).toContain('Onay:')
  })

  it('başlık, geri bağlantısı ve ana bölge vardır', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    expect(w.get('main h1').text()).toBe('Gönderim takibi')
    expect(w.findComponent(RouterLinkStub).props('to')).toBe(`/campaigns/${ID}`)
    expect(w.text()).toContain('Kampanyaya dön')
  })

  it('brief HTML olarak yorumlanmaz', async () => {
    handlers[detailPath(ID)] = () =>
      jsonResponse(200, {
        ...campaignBody({ id: ID, status: 'approved', brief: '<img src=x onerror=alert(1)>' }),
        approved_content_version: 1,
        approved_at: '2026-11-19T10:00:00Z',
      })
    const w = await mountView()

    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('kampanya bulunamazsa hata gösterir; yeniden deneme önermez ve canlı takibe bağlanmaz', async () => {
    handlers[detailPath(ID)] = () => errorResponse(404, 'CAMPAIGN_NOT_FOUND')
    const w = await mountView()

    const alert = w.get('[data-testid="load-error"]')
    expect(alert.attributes('role')).toBe('alert')
    expect(alert.text()).toContain('Kampanya bulunamadı.')
    expect(w.find('[data-testid="load-error"] button').exists()).toBe(false)
    expect(callsTo(eventsPath(ID))).toHaveLength(0)
  })

  it('ağ hatasında "Yeniden dene" ile yükler ve canlı takibe bağlanır', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()
    expect(w.get('[data-testid="load-error"]').text()).toContain('Sunucuya ulaşılamadı')

    await w.get('[data-testid="load-error"] button').trigger('click')
    await flushPromises()

    expect(w.find('[data-testid="load-error"]').exists()).toBe(false)
    expect(w.find('[data-testid="campaign-status"]').exists()).toBe(true)
    expect(callsTo(eventsPath(ID))).toHaveLength(1)
  })
})

describe('onaylanmamış kampanya', () => {
  it('taslak kampanyada gönderim olmadığını söyler ve canlı takibe bağlanmaz', async () => {
    handlers[detailPath(ID)] = () => jsonResponse(200, campaignBody({ id: ID, status: 'draft' }))
    const w = await mountView()

    expect(w.get('[data-testid="not-approved"]').text()).toContain('henüz onaylanmadı')
    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Taslak')
    expect(w.find('[data-testid="connection-status"]').exists()).toBe(false)
    expect(w.find('[data-testid="total-messages"]').exists()).toBe(false)
    expect(callsTo(eventsPath(ID))).toHaveLength(0)
  })
})

describe('canlı durum', () => {
  it('bağlanır; ilk görüntüye kadar bekleme göstergesi, sonra gerçek sayılar', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()

    expect(w.get('[data-testid="connection-status"]').text()).toContain('bağlanılıyor')
    expect(w.find('[data-testid="waiting-first-snapshot"]').exists()).toBe(true)
    expect(w.find('[data-testid="total-messages"]').exists()).toBe(false)

    await push(0, snap({ queued: 3, retrying: 1, sending: 2, sent: 40, failed: 2, skipped: 1 }))

    expect(w.find('[data-testid="waiting-first-snapshot"]').exists()).toBe(false)
    expect(count(w, 'queued')).toBe('3')
    expect(count(w, 'retrying')).toBe('1')
    expect(count(w, 'sending')).toBe('2')
    expect(count(w, 'sent')).toBe('40')
    expect(count(w, 'failed')).toBe('2')
    expect(count(w, 'skipped')).toBe('1')
    expect(w.get('[data-testid="total-messages"]').text()).toBe('49')
    expect(w.get('[data-testid="connection-status"]').text()).toContain('Canlı')
    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Gönderiliyor') // canlı durumdan
  })

  it('her görüntü sayıları değiştirir, üzerine eklemez', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()
    await push(0, snap({ queued: 10 }))
    await push(0, snap({ queued: 4, sent: 6 }))

    expect(count(w, 'queued')).toBe('4')
    expect(count(w, 'sent')).toBe('6')
    expect(w.get('[data-testid="total-messages"]').text()).toBe('10')
  })

  it('başarısızlık nedenlerini gerçek sayılarla açıklar', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()
    await push(0, snap({ sent: 5, failed: 3 }, { failures: { exhausted: 2, rejected: 1 } }))

    expect(w.get('[data-testid="note-exhausted"]').text()).toContain(
      '2 mesaj: Gönderim sonucu doğrulanamadı; deneme sınırı doldu',
    )
    expect(w.get('[data-testid="note-rejected"]').text()).toContain('1 mesaj')
  })

  it('tamamlanınca durumu ve gerçek dağılımı gösterir, canlı takibi kapatır', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()
    await push(0, snap({ sending: 2, sent: 5 }))
    await push(
      0,
      snap(
        { sent: 6, failed: 1 },
        { status: 'completed', failures: { exhausted: 1, rejected: 0 } },
      ),
    )

    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Tamamlandı')
    expect(w.get('[data-testid="completed-note"]').text()).toContain('Gönderim tamamlandı')
    expect(w.get('[data-testid="completed-note"]').text()).toContain('engellemez')
    // Sonuç özeti: gönderilen ve başarısız sayıları kısa açıklamayla.
    const lines = w.get('[data-testid="completion-lines"]').text()
    expect(lines).toContain('Gönderildi: 6')
    expect(lines).toContain('Başarısız: 1')
    expect(w.get('[data-testid="completion-headline"]').text()).toBe(
      "7 mesajın 6'sı gönderildi, 1'i başarısız.",
    )
    // Başarısız için ne yapılabileceği yazılı.
    const advice = w.get('[data-testid="completion-advice"]').text()
    expect(advice).toContain('Ne yapabilirsiniz?')
    expect(advice).toContain('teslim edilmiş olabilir')
    expect(count(w, 'sent')).toBe('6')
    expect(count(w, 'failed')).toBe('1')
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('100')
    expect(w.get('[data-testid="connection-status"]').text()).toContain('Canlı takip kapatıldı')
    expect(stream(0).signal?.aborted).toBe(true)
    expect(callsTo(eventsPath(ID))).toHaveLength(1) // bir daha bağlanmaz
  })

  it('zaten tamamlanmış kampanyada son durumu bir kez alır ve kapatır', async () => {
    handlers[detailPath(ID)] = () => approved(ID, { status: 'completed' })
    const w = await mountView()
    await push(0, snap({ sent: 9, failed: 1 }, { status: 'completed' }))

    expect(count(w, 'sent')).toBe('9')
    expect(w.find('[data-testid="completed-note"]').exists()).toBe(true)
    expect(stream(0).signal?.aborted).toBe(true)
  })
})

describe('bağlantı hataları', () => {
  beforeEach(() => {
    // Yalnızca bekleme ve takılma zamanlayıcıları sahte; promise ve DOM güncellemeleri gerçek.
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  })

  it('bağlantı koparsa son bilinen sayıları tutar, yeniden bağlanıldığını söyler ve toparlanınca yeniler', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()
    await push(0, snap({ sent: 5 }))

    stream(0).sse.fail(new TypeError('network error'))
    await flushPromises()
    expect(w.get('[data-testid="connection-status"]').text()).toContain(
      'yeniden bağlanılıyor (deneme 1)',
    )
    expect(count(w, 'sent')).toBe('5') // son bilinen durum

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    expect(callsTo(eventsPath(ID))).toHaveLength(2)
    await push(1, snap({ sent: 8 }))

    expect(w.get('[data-testid="connection-status"]').text()).toContain('Canlı')
    expect(count(w, 'sent')).toBe('8')
  })

  it('bağlantı kurulamazsa sayıları göstermeden hata verir ve elle yeniden bağlanılabilir', async () => {
    handlers[detailPath(ID)] = () => approved()
    handlers[eventsPath(ID)] = () => new Response('<html>Bad Gateway</html>', { status: 502 })
    const w = await mountView()

    // 502 "beklenmeyen yanıt" yeniden denenebilir: 5 ardışık başarısızlıktan sonra vazgeçilir.
    for (const wait of [1000, 2000, 4000, 8000]) {
      await vi.advanceTimersByTimeAsync(wait)
      await flushPromises()
    }

    const failed = w.get('[data-testid="connection-failed"]')
    expect(failed.attributes('role')).toBe('alert')
    expect(failed.text()).toContain('beklenmeyen bir yanıt')
    expect(w.find('[data-testid="total-messages"]').exists()).toBe(false)
    expect(w.find('[data-testid="waiting-first-snapshot"]').exists()).toBe(false)
    expect(callsTo(eventsPath(ID))).toHaveLength(5)

    // Sunucu düzelince "Yeniden bağlan" çalışır.
    delete handlers[eventsPath(ID)]
    await failed.get('button').trigger('click')
    await flushPromises()
    await push(0, snap({ sent: 3 }))
    expect(w.find('[data-testid="connection-failed"]').exists()).toBe(false)
    expect(count(w, 'sent')).toBe('3')
  })

  it('sunucu kampanyayı bulamazsa (olay akışı 404) yeniden denemeden hata gösterir', async () => {
    handlers[detailPath(ID)] = () => approved()
    handlers[eventsPath(ID)] = () => errorResponse(404, 'CAMPAIGN_NOT_FOUND')
    const w = await mountView()

    expect(w.get('[data-testid="connection-failed"]').text()).toContain('Kampanya bulunamadı.')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(callsTo(eventsPath(ID))).toHaveLength(1)
  })
})

describe('sayfa değişimi ve temizlik', () => {
  it('başka kampanyaya geçince eski akış kapanır, eski sayılar silinir, yenisi yüklenir', async () => {
    handlers[detailPath(ID)] = () => approved(ID)
    handlers[detailPath(OTHER)] = () => approved(OTHER)
    const w = await mountView(ID)
    await push(0, snap({ sent: 40 }))
    expect(count(w, 'sent')).toBe('40')

    await w.setProps({ id: OTHER })
    await flushPromises()

    expect(stream(0).signal?.aborted).toBe(true)
    expect(w.text()).toContain(`Brief ${OTHER.slice(0, 4)}`)
    expect(w.find('[data-testid="total-messages"]').exists()).toBe(false) // eski kampanyanın sayıları yok
    expect(callsTo(eventsPath(OTHER))).toHaveLength(1)

    await push(1, snap({ sent: 2 }))
    expect(count(w, 'sent')).toBe('2')
    expect(w.findComponent(RouterLinkStub).props('to')).toBe(`/campaigns/${OTHER}`)
  })

  it('eski kampanyadan geç gelen olay yeni ekranı değiştirmez', async () => {
    // A'nın akışı iptal edilse bile veri yollamayı sürdürür (ağda gecikmiş olay).
    let late: ReadableStreamDefaultController<Uint8Array> | undefined
    handlers[eventsPath(ID)] = () =>
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            late = controller
          },
        }),
        { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
      )
    handlers[detailPath(ID)] = () => approved(ID)
    handlers[detailPath(OTHER)] = () => approved(OTHER)
    const w = await mountView(ID)

    await w.setProps({ id: OTHER })
    await flushPromises()
    await push(0, snap({ sent: 2 })) // B'nin ilk (ve tek) bağlantısı
    expect(count(w, 'sent')).toBe('2')

    late?.enqueue(new TextEncoder().encode(snap({ sent: 99 }, { status: 'completed' })))
    await flushPromises()

    expect(count(w, 'sent')).toBe('2')
    expect(w.get('[data-testid="campaign-status"]').text()).toBe('Gönderiliyor')
    expect(w.find('[data-testid="completed-note"]').exists()).toBe(false)
  })

  it('yavaş yüklenen eski kampanyanın yanıtı yeni kampanyayı ezmez', async () => {
    const slow = deferred<Response>()
    handlers[detailPath(ID)] = () => slow.promise
    handlers[detailPath(OTHER)] = () => approved(OTHER)
    const w = mount(DeliveryDashboardView, {
      props: { id: ID },
      global: { stubs: { RouterLink: RouterLinkStub } },
    })
    mounted.push(w)
    await flushPromises()

    await w.setProps({ id: OTHER })
    await flushPromises()
    expect(w.text()).toContain(`Brief ${OTHER.slice(0, 4)}`)

    slow.resolve(approved(ID)) // eski kampanyanın geç gelen yanıtı
    await flushPromises()

    expect(w.text()).toContain(`Brief ${OTHER.slice(0, 4)}`)
    expect(w.text()).not.toContain(`Brief ${ID.slice(0, 4)}`)
    expect(callsTo(eventsPath(ID))).toHaveLength(0) // eski kampanya için akış açılmadı
    expect(callsTo(eventsPath(OTHER))).toHaveLength(1)
  })

  it('ekran kapanınca canlı takip bağlantısı kapanır', async () => {
    handlers[detailPath(ID)] = () => approved()
    const w = await mountView()
    await push(0, snap({ sent: 1 }))

    w.unmount()
    await flushPromises()

    expect(stream(0).signal?.aborted).toBe(true)
  })

  it('kampanya yüklenirken ekran kapanırsa istek iptal edilir ve akış açılmaz', async () => {
    let signal: AbortSignal | null | undefined
    fetchMock.mockImplementation(async (input, init) => {
      calls.push(String(input))
      signal = init?.signal
      return new Promise<Response>(() => undefined) // hiç dönmeyen yükleme
    })
    const w = await mountView()
    w.unmount()
    await flushPromises()

    expect(signal?.aborted).toBe(true)
    expect(callsTo(eventsPath(ID))).toHaveLength(0)
  })
})
