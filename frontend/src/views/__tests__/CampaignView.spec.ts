import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { RouterLinkStub, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import CampaignView from '../CampaignView.vue'
import {
  campaignBody,
  contentBody,
  controlledSse,
  errorResponse,
  jsonResponse,
  sseEvent,
  sseResponse,
  type ControlledSse,
} from '@/__tests__/helpers'

const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'
const fetchMock = vi.fn<typeof fetch>()

type Handler = (init?: RequestInit) => Response | Promise<Response>
let handlers: Record<string, Handler> = {}
const calls: { method: string; path: string; init?: RequestInit }[] = []

function routes(map: Record<string, Handler>) {
  handlers = { ...handlers, ...map }
}
const callsTo = (key: string) => calls.filter((c) => `${c.method} ${c.path}` === key)

beforeEach(() => {
  handlers = {}
  calls.length = 0
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    const method = init?.method ?? 'GET'
    const path = String(input)
    calls.push({ method, path, init })
    const handler = handlers[`${method} ${path}`]
    if (!handler) throw new Error(`beklenmeyen istek: ${method} ${path}`)
    return handler(init)
  })
  vi.stubGlobal('fetch', fetchMock)
})
// Kaydedilmemiş değişiklik uyarısı `beforeunload` dinleyicisini pencereye kurar: bileşenler
// kapatılmazsa bir testin kirli taslağı sonraki testlere sızar.
let mounted: VueWrapper[] = []
afterEach(() => {
  mounted.forEach((w) => w.unmount())
  mounted = []
  vi.unstubAllGlobals()
})

const GET = `GET /api/campaigns/${ID}`
const GENERATE = `POST /api/campaigns/${ID}/content`
const SAVE = `PUT /api/campaigns/${ID}/content`
const APPROVE = `POST /api/campaigns/${ID}/approve`

const detail = (extra: Record<string, unknown> = {}) =>
  jsonResponse(200, { ...campaignBody(), ...extra })

async function mountView(): Promise<VueWrapper> {
  const w = mount(CampaignView, {
    props: { id: ID },
    global: { stubs: { RouterLink: RouterLinkStub } },
  })
  mounted.push(w)
  await flushPromises()
  return w
}

const sms = (w: VueWrapper) => w.get('#sms_text').element as HTMLTextAreaElement
const subject = (w: VueWrapper) => w.get('#email_subject').element as HTMLInputElement
const body = (w: VueWrapper) => w.get('#email_body').element as HTMLTextAreaElement
const button = (w: VueWrapper, text: string) =>
  w.findAll('button').find((b) => b.text().includes(text))
const click = async (w: VueWrapper, text: string) => {
  const b = button(w, text)
  if (!b) throw new Error(`"${text}" düğmesi yok: ${w.text()}`)
  await b.trigger('click')
  await flushPromises()
}
const confirmApproval = async (w: VueWrapper, version: number) => {
  await click(w, `Onayla (sürüm ${version})`)
  await click(w, 'Onayla ve kuyruğa al')
}

const approvedBody = {
  id: ID,
  status: 'approved',
  approved_content_version: 1,
  approved_at: '2026-11-18T11:00:00Z',
  queued: { sms: 133, email: 150, total: 283 },
}

const feedbackText = (w: VueWrapper) => w.get('[data-testid="action-feedback"]').text()

describe('işlem geri bildirimi', () => {
  it('üretim bitince "Üretildi · sürüm 1" der', async () => {
    routes({
      [GET]: () => detail(),
      [GENERATE]: () =>
        sseResponse([
          sseEvent('delta', { field: 'sms_text', text: 'Merhaba', replace: true }),
          sseEvent('validated', contentBody({ content_version: 1 })),
        ]),
    })
    const w = await mountView()
    expect(w.find('[data-testid="action-feedback"]').exists()).toBe(false) // henüz editör yok

    await click(w, 'İçerik üret')

    expect(feedbackText(w)).toBe('Üretildi · sürüm 1')
  })

  it('kaydedince "Kaydedildi · sürüm 2" der; metin değiştirilince geri bildirim kaybolur', async () => {
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [SAVE]: () => jsonResponse(200, contentBody({ content_version: 2, source: 'human' })),
    })
    const w = await mountView()
    expect(feedbackText(w)).toBe('') // açılışta geri bildirim yok

    await w.get('#sms_text').setValue('Elle düzenlenmiş metin')
    await click(w, 'Yeni sürüm olarak kaydet')
    expect(feedbackText(w)).toBe('Kaydedildi · sürüm 2')

    await w.get('#sms_text').setValue('Yine değişti')
    expect(feedbackText(w)).toBe('')
    expect(w.text()).toContain('Kaydedilmemiş değişiklik var.')
  })

  it('geri bildirim ekran okuyucuya bildirilen bir durum bölgesindedir', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const w = await mountView()
    expect(w.get('[data-testid="action-feedback"]').attributes('role')).toBe('status')
  })

  it('kayıt başarısız olursa başarı mesajı çıkmaz', async () => {
    routes({
      [GET]: () => detail({ content: contentBody() }),
      [SAVE]: () => errorResponse(422, 'CONTENT_INVALID'),
    })
    const w = await mountView()
    await w.get('#sms_text').setValue('Geçersiz olabilir')
    await click(w, 'Yeni sürüm olarak kaydet')

    expect(feedbackText(w)).toBe('')
  })

  it('kaydetme sürerken düğme kapalıdır ve çift tıklama tek istek yapar', async () => {
    let finish: (r: Response) => void = () => {}
    routes({
      [GET]: () => detail({ content: contentBody() }),
      [SAVE]: () => new Promise<Response>((resolve) => (finish = resolve)),
    })
    const w = await mountView()
    await w.get('#sms_text').setValue('Yeni metin')

    await click(w, 'Yeni sürüm olarak kaydet')
    expect(button(w, 'Kaydediliyor…')?.attributes('disabled')).toBeDefined()
    await button(w, 'Kaydediliyor…')?.trigger('click')
    await flushPromises()
    expect(callsTo(SAVE)).toHaveLength(1)
    expect(feedbackText(w)).toBe('') // işlem sürerken başarı mesajı yok

    finish(jsonResponse(200, contentBody({ content_version: 2, source: 'human' })))
    await flushPromises()
    expect(feedbackText(w)).toBe('Kaydedildi · sürüm 2')
  })

  it('onay sürerken düğme "Onaylanıyor…" der ve kapalıdır', async () => {
    let finish: (r: Response) => void = () => {}
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [APPROVE]: () => new Promise<Response>((resolve) => (finish = resolve)),
    })
    const w = await mountView()
    await confirmApproval(w, 1)

    expect(button(w, 'Onaylanıyor…')?.attributes('disabled')).toBeDefined()
    finish(jsonResponse(200, approvedBody))
    await flushPromises()
    expect(w.find('[data-testid="approved"]').exists()).toBe(true)
  })
})

describe('mesaj önizlemesi', () => {
  it('düzenleyicinin altında örnek adla canlı önizleme gösterir ve örnek olduğunu belirtir', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const w = await mountView()
    const preview = w.get('[data-testid="editor-preview"]')

    expect(preview.get('[data-testid="preview-sms"]').text()).toBe(
      'Merhaba Ayşe, %30 indirim seni bekliyor!',
    )
    expect(preview.get('[data-testid="preview-note"]').text()).toContain('Örnek önizleme')

    await w.get('#sms_text').setValue('Selam {{first_name}}, yeni metin')
    expect(preview.get('[data-testid="preview-sms"]').text()).toBe('Selam Ayşe, yeni metin')
  })

  it('içerik yokken önizleme çıkmaz', async () => {
    routes({ [GET]: () => detail() })
    const w = await mountView()
    expect(w.find('[data-testid="editor-preview"]').exists()).toBe(false)
  })

  it('onay özeti kaydedilmiş sürümü gösterir (kaydedilmemiş taslağı değil)', async () => {
    routes({ [GET]: () => detail({ content: contentBody({ content_version: 4 }) }) })
    const w = await mountView()
    await w.get('#sms_text').setValue('Taslak, henüz kaydedilmedi')

    const aside = w.get('aside')
    expect(aside.get('[data-testid="approve-version"]').text()).toBe('Sürüm 4 onaylanacak.')
    expect(aside.get('[data-testid="preview-sms"]').text()).toBe(
      'Merhaba Ayşe, %30 indirim seni bekliyor!',
    )
  })
})

describe('kaydedilmemiş değişiklik uyarısı', () => {
  // Gerçek yönlendirici: gezinme korumaları ancak RouterView içinde çalışır.
  async function mountRouted() {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: defineComponent({ render: () => h('div', 'ana sayfa') }) },
        { path: '/campaigns/:id', component: CampaignView, props: true },
      ],
    })
    await router.push(`/campaigns/${ID}`)
    const w = mount(defineComponent({ render: () => h(RouterView) }), {
      global: { plugins: [router] },
    })
    mounted.push(w)
    await flushPromises()
    return { w, router }
  }

  it('metni değiştirdikten sonra başka sayfaya gidilirken uyarır; vazgeçilirse metin korunur', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { w, router } = await mountRouted()

    await w.get('#sms_text').setValue('Yeni metin')
    await router.push('/')
    await flushPromises()

    expect(confirm).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.path).toBe(`/campaigns/${ID}`)
    expect((w.get('#sms_text').element as HTMLTextAreaElement).value).toBe('Yeni metin')
    confirm.mockRestore()
  })

  it('"Yeni kampanya" bağlantısı da kaydedilmemiş değişiklikte uyarır', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { w, router } = await mountRouted()

    await w.get('#sms_text').setValue('Yeni metin')
    await w.get('[data-testid="nav-back"]').trigger('click')
    await flushPromises()

    expect(confirm).toHaveBeenCalledTimes(1)
    expect(router.currentRoute.value.path).toBe(`/campaigns/${ID}`)
    confirm.mockRestore()
  })

  it('kullanıcı onaylarsa sayfadan ayrılır', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const { w, router } = await mountRouted()

    await w.get('#sms_text').setValue('Yeni metin')
    await router.push('/')
    await flushPromises()

    expect(router.currentRoute.value.path).toBe('/')
    confirm.mockRestore()
  })

  it('değişiklik yoksa veya kaydedildiyse hiçbir şey sormaz', async () => {
    routes({
      [GET]: () => detail({ content: contentBody() }),
      [SAVE]: () => jsonResponse(200, contentBody({ content_version: 2, source: 'human' })),
    })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { w, router } = await mountRouted()

    await w.get('#sms_text').setValue('Kaydedilecek metin')
    await w
      .findAll('button')
      .find((b) => b.text().includes('Yeni sürüm olarak kaydet'))
      ?.trigger('click')
    await flushPromises()
    await router.push('/')
    await flushPromises()

    expect(confirm).not.toHaveBeenCalled()
    expect(router.currentRoute.value.path).toBe('/')
    confirm.mockRestore()
  })

  it('metin değiştirilmişken sekme kapatılırsa tarayıcı uyarısı istenir', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const { w } = await mountRouted()
    const clean = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(clean)
    expect(clean.defaultPrevented).toBe(false)

    await w.get('#sms_text').setValue('Yeni metin')
    const dirty = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(dirty)
    expect(dirty.defaultPrevented).toBe(true)
  })
})

const stepStates = (w: VueWrapper) =>
  w.findAll('[data-testid="campaign-steps"] li').map((li) => li.attributes('data-status'))

describe('üst bölüm ve adım göstergesi', () => {
  it('başlık, kısa açıklama ve durum rozetini gösterir', async () => {
    routes({ [GET]: () => detail() })
    const w = await mountView()

    expect(w.get('h1').text()).toBe('Kampanya')
    expect(w.get('[data-testid="page-description"]').text()).toContain('onaylayın')
    expect(w.get('[data-testid="campaign-state"]').text()).toBe('Taslak')
  })

  it('yüklenirken ve hata durumunda adım göstergesi ve rozet yoktur', async () => {
    routes({ [GET]: () => errorResponse(404, 'CAMPAIGN_NOT_FOUND') })
    const w = await mountView()

    expect(w.find('[data-testid="campaign-steps"]').exists()).toBe(false)
    expect(w.find('[data-testid="campaign-state"]').exists()).toBe(false)
  })

  it('içeriksiz taslakta içerik adımı sürer', async () => {
    routes({ [GET]: () => detail() })
    const w = await mountView()
    expect(stepStates(w)).toEqual(['done', 'current', 'upcoming', 'upcoming'])
  })

  it('kaydedilmiş içerik varken onay adımı sürer; kaydedilmemiş değişiklik içeriğe geri döner', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const w = await mountView()
    expect(stepStates(w)).toEqual(['done', 'done', 'current', 'upcoming'])

    await w.get('#sms_text').setValue('Değiştirilmiş metin')
    expect(stepStates(w)).toEqual(['done', 'current', 'upcoming', 'upcoming'])
  })

  it('onaylanınca gönderim adımı sürer ve rozet güncellenir', async () => {
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [APPROVE]: () => jsonResponse(200, approvedBody),
    })
    const w = await mountView()
    await confirmApproval(w, 1)

    expect(stepStates(w)).toEqual(['done', 'done', 'done', 'current'])
    expect(w.get('[data-testid="campaign-state"]').text()).toBe('Onaylandı, kuyrukta')
  })
})

describe('iki sütunlu düzen', () => {
  it('düzenleyici ana sütunda, kitle ve onay özeti yan sütundadır', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const w = await mountView()
    const aside = w.get('aside')

    expect(aside.attributes('aria-label')).toBe('Kitle ve onay özeti')
    expect(aside.find('[data-testid="total"]').exists()).toBe(true)
    expect(aside.find('#segment-title').exists()).toBe(true)
    expect(aside.find('#approve-title').exists()).toBe(true)
    expect(aside.find('#sms_text').exists()).toBe(false)
    expect(w.find('main #sms_text').exists()).toBe(true)
  })

  it('yan sütundaki kitle sayıları kompakt görünümdedir', async () => {
    routes({ [GET]: () => detail() })
    const w = await mountView()
    expect(w.get('aside').find('[data-testid="audience-compact"]').exists()).toBe(true)
  })

  it('dar ekranda (tek sütun) düzenleyici yan sütundan önce gelir', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const w = await mountView()
    const editor = w.get('#sms_text').element
    const aside = w.get('aside').element
    // Belge sırasında düzenleyici önce: telefonda içerik, sonra kitle ve onay.
    expect(editor.compareDocumentPosition(aside) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('onaylı kampanyada onay bilgisi yan sütundadır; gönderim bağlantısı "İleri"dir', async () => {
    routes({ [GET]: () => detail({ status: 'approved', content: contentBody() }) })
    const w = await mountView()
    const aside = w.get('aside')

    expect(aside.find('[data-testid="approved"]').exists()).toBe(true)
    expect(aside.find('#approve-title').exists()).toBe(false)
    expect(aside.find('a, button').exists()).toBe(false) // yan sütunda yinelenen bağlantı yok
    expect(w.find('[data-testid="step-nav"] [data-testid="delivery-link"]').exists()).toBe(true)
  })
})

describe('Geri ve İleri', () => {
  const navLink = (w: VueWrapper, id: string) =>
    w.findAllComponents(RouterLinkStub).find((l) => l.attributes('data-testid') === id)

  it('sol düğme "Geri" değil "Yeni kampanya" der (önceki aşamaya dönmez); "İleri" onaya kadar kapalıdır', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const w = await mountView()

    const back = navLink(w, 'nav-back')
    expect(back?.props('to')).toBe('/')
    expect(back?.text()).toBe('+ Yeni kampanya')
    expect(back?.text()).not.toContain('Geri') // kullanıcı mevcut kampanyaya döndüğünü sanmasın
    expect(back?.attributes('title')).toContain('Bu kampanya olduğu gibi kayıtlı kalır')
    const next = w.get('[data-testid="nav-next-disabled"]')
    expect(next.attributes('disabled')).toBeDefined()
    expect(next.text()).toBe('İleri: Gönderimi izle')
    expect(w.get('#step-nav-next-hint').text()).toBe('Önce içeriği onaylayın.')
    expect(navLink(w, 'delivery-link')).toBeUndefined()
  })

  it('onaylanınca "İleri" gönderim takibine açılır', async () => {
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [APPROVE]: () => jsonResponse(200, approvedBody),
    })
    const w = await mountView()
    await confirmApproval(w, 1)

    const next = navLink(w, 'delivery-link')
    expect(next?.props('to')).toBe(`/campaigns/${ID}/delivery`)
    expect(next?.text()).toBe('İleri: Gönderimi izle')
    expect(w.find('[data-testid="nav-next-disabled"]').exists()).toBe(false)
  })

  it('onaylı kampanya yüklenince "İleri" baştan açıktır', async () => {
    routes({ [GET]: () => detail({ status: 'sending', content: contentBody() }) })
    const w = await mountView()
    expect(navLink(w, 'delivery-link')?.props('to')).toBe(`/campaigns/${ID}/delivery`)
  })

  it('kampanya yüklenemezse "Geri" yine kullanılabilir', async () => {
    routes({ [GET]: () => errorResponse(404, 'CAMPAIGN_NOT_FOUND') })
    const w = await mountView()
    expect(navLink(w, 'nav-back')?.props('to')).toBe('/')
    expect(w.find('[data-testid="nav-next"]').exists()).toBe(false)
  })

  it('dar ekranda alta yapışır (ana eylemlere her zaman ulaşılır)', async () => {
    routes({ [GET]: () => detail({ content: contentBody() }) })
    const w = await mountView()
    expect(w.get('[data-testid="step-nav"]').classes()).toContain('sticky')
  })
})

describe('yükleme', () => {
  it('içeriği olmayan taslağı gösterir: kitle, koşullar ve üretim/elle yazma seçenekleri', async () => {
    routes({ [GET]: () => detail() })
    const w = await mountView()

    expect(w.get('[data-testid="total"]').text()).toBe('190')
    expect(w.text()).toContain('Şehir: İstanbul')
    expect(w.text()).toContain('Taslak')
    expect(button(w, 'İçerik üret')).toBeDefined()
    expect(button(w, 'İçeriği elle yaz')).toBeDefined()
    expect(w.find('#sms_text').exists()).toBe(false) // düzenleyici üretimden önce yok
    expect(w.find('#approve-title').exists()).toBe(false) // kaydedilmiş sürüm yok
    expect(callsTo(GET)).toHaveLength(1)
  })

  it('sayfa yenilenince son doğrulanmış içeriği geri yükler ve onaya hazırdır', async () => {
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 3, source: 'human' }) }),
    })
    const w = await mountView()

    expect(w.get('[data-testid="version"]').text()).toBe('Sürüm 3 · Elle düzenlendi')
    expect(sms(w).value).toBe(contentBody().sms_text)
    expect(subject(w).value).toBe(contentBody().email_subject)
    expect(button(w, 'Onayla (sürüm 3)')?.attributes('disabled')).toBeUndefined()
    expect(button(w, 'Yeniden üret')).toBeDefined()
  })

  it('kampanya bulunamazsa hata gösterir (yeniden deneme önerilmez)', async () => {
    routes({ [GET]: () => errorResponse(404, 'CAMPAIGN_NOT_FOUND') })
    const w = await mountView()

    expect(w.get('[data-testid="error-panel"]').text()).toContain('Kampanya bulunamadı.')
    expect(w.find('[data-testid="error-retry"]').exists()).toBe(false)
  })

  it('ağ hatasında yeniden deneyince yükler', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('failed'))
    routes({ [GET]: () => detail() })
    const w = await mountView()
    expect(w.get('[data-testid="error-panel"]').text()).toContain('Sunucuya ulaşılamadı')

    await w.get('[data-testid="error-retry"]').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Şehir: İstanbul')
  })

  it('metinleri HTML olarak yorumlamaz', async () => {
    routes({
      [GET]: () =>
        jsonResponse(200, {
          ...campaignBody({ brief: '<img src=x onerror=alert(1)>' }),
          content: contentBody({ email_body: '<script>alert(1)</script>' }),
        }),
    })
    const w = await mountView()

    expect(w.find('img').exists()).toBe(false)
    expect(w.find('script').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })
})

describe('içerik üretimi (SSE)', () => {
  let sse: ControlledSse

  it('akış sürerken metni canlı gösterir, bitince doğrulanmış sürümle değiştirir', async () => {
    routes({
      [GET]: () => detail(),
      [GENERATE]: (init) => (sse = controlledSse(init?.signal)).response,
    })
    const w = await mountView()
    await click(w, 'İçerik üret')

    sse.push(sseEvent('delta', { field: 'sms_text', text: 'Mer', replace: true }))
    await vi.waitFor(() => expect(sms(w).value).toBe('Mer'))
    // Akış sürerken: geçici önizleme, düzenleme ve onay kapalı, tekrar üretim engelli.
    expect(sms(w).readOnly).toBe(true)
    expect(button(w, 'Üretiliyor…')?.attributes('disabled')).toBeDefined()
    expect(w.find('#approve-title').exists()).toBe(false)
    expect(w.text()).toContain('Gördüğünüz metin geçicidir')

    sse.push(sseEvent('delta', { field: 'sms_text', text: 'haba', replace: false }))
    sse.push(sseEvent('delta', { field: 'email_subject', text: 'Konu', replace: true }))
    await vi.waitFor(() => expect(sms(w).value).toBe('Merhaba'))
    expect(subject(w).value).toBe('Konu')

    // Doğrulanmış içerik önizlemeyi tümüyle değiştirir (eski metne eklenmez).
    sse.push(
      sseEvent('validated', contentBody({ sms_text: 'Kesin SMS', email_subject: 'Kesin konu' })),
    )
    await vi.waitFor(() =>
      expect(w.get('[data-testid="version"]').text()).toContain('Sürüm 1 · Yapay zekâ'),
    )
    expect(sms(w).value).toBe('Kesin SMS')
    expect(subject(w).value).toBe('Kesin konu')
    expect(sms(w).readOnly).toBe(false)
    expect(button(w, 'Onayla (sürüm 1)')).toBeDefined()
    expect(callsTo(GENERATE)).toHaveLength(1)
  })

  it('çift tıklama tek istek yapar', async () => {
    routes({
      [GET]: () => detail(),
      [GENERATE]: (init) => (sse = controlledSse(init?.signal)).response,
    })
    const w = await mountView()
    const generate = button(w, 'İçerik üret')!
    void generate.trigger('click')
    void generate.trigger('click')
    await flushPromises()

    expect(callsTo(GENERATE)).toHaveLength(1)
  })

  it('doğrulanamayan üretimde sebepleri alanın altında gösterir ve metni düzeltmeye açar; onay yoktur', async () => {
    routes({
      [GET]: () => detail(),
      [GENERATE]: () =>
        sseResponse([
          sseEvent('delta', { field: 'sms_text', text: 'x'.repeat(200), replace: true }),
          sseEvent('error', {
            code: 'CONTENT_INVALID',
            message: 'm',
            reasons: [
              { code: 'TOO_LONG', field: 'sms_text' },
              { code: 'UNKNOWN_VARIABLE', field: 'email_body', text: 'city' },
              { code: 'MALFORMED_OUTPUT' },
            ],
            partial: {
              sms_text: 'x'.repeat(200),
              email_subject: 'Konu',
              email_body: 'Selam {{city}}',
            },
          }),
        ]),
    })
    const w = await mountView()
    await click(w, 'İçerik üret')

    expect(w.get('[data-testid="error-panel"]').text()).toContain('İçerik doğrulanamadı.')
    expect(w.get('#sms-problems').text()).toContain('160')
    expect(w.get('#body-problems').text()).toContain('{{city}}')
    expect(w.text()).toContain('Üretilen içerik beklenen biçimde değil.') // alansız sebep
    expect(w.text()).toContain('henüz kaydedilmedi')
    expect(sms(w).value).toBe('x'.repeat(200))
    expect(sms(w).readOnly).toBe(false)
    expect(w.find('#approve-title').exists()).toBe(false) // doğrulanmamış metin onaylanamaz
    expect(w.find('[data-testid="version"]').exists()).toBe(false)
    expect(button(w, 'Yeni sürüm olarak kaydet')?.attributes('disabled')).toBeUndefined()
  })

  it('doğrulanamayan metni düzeltip kaydedince sürüm olur ve onaya açılır', async () => {
    routes({
      [GET]: () => detail(),
      [GENERATE]: () =>
        sseResponse([
          sseEvent('error', {
            code: 'CONTENT_INVALID',
            message: 'm',
            reasons: [{ code: 'TOO_LONG', field: 'sms_text' }],
            partial: { sms_text: 'x'.repeat(200), email_subject: 'Konu', email_body: 'Gövde' },
          }),
        ]),
      [SAVE]: () =>
        jsonResponse(
          200,
          contentBody({ content_version: 1, source: 'human', sms_text: 'Kısa SMS' }),
        ),
    })
    const w = await mountView()
    await click(w, 'İçerik üret')
    await w.get('#sms_text').setValue('Kısa SMS')
    await click(w, 'Yeni sürüm olarak kaydet')

    expect(JSON.parse(String(callsTo(SAVE)[0]!.init?.body))).toEqual({
      sms_text: 'Kısa SMS',
      email_subject: 'Konu',
      email_body: 'Gövde',
    })
    expect(w.get('[data-testid="version"]').text()).toBe('Sürüm 1 · Elle düzenlendi')
    expect(w.text()).not.toContain('henüz kaydedilmedi')
    expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
    expect(button(w, 'Onayla (sürüm 1)')).toBeDefined()
  })

  it.each([
    ['LLM_TIMEOUT', 'zaman aşımına uğradı'],
    ['LLM_RATE_LIMITED', 'yoğun'],
  ])('%s hatasında önceki kayıtlı sürüm korunur ve yeniden üretilebilir', async (code, text) => {
    let attempt = 0
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [GENERATE]: () =>
        ++attempt === 1
          ? sseResponse([
              sseEvent('delta', { field: 'sms_text', text: 'geçici', replace: true }),
              sseEvent('error', { code, message: 'm' }),
            ])
          : sseResponse([
              sseEvent('validated', contentBody({ content_version: 2, sms_text: 'Yeni' })),
            ]),
    })
    const w = await mountView()
    await click(w, 'Yeniden üret')

    expect(w.get('[data-testid="error-panel"]').text()).toContain(text)
    expect(sms(w).value).toBe(contentBody().sms_text) // geçici metin silindi, kayıtlı sürüm geri geldi
    expect(w.get('[data-testid="version"]').text()).toContain('Sürüm 1')
    expect(button(w, 'Onayla (sürüm 1)')?.attributes('disabled')).toBeUndefined()

    await w.get('[data-testid="error-retry"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-testid="version"]').text()).toContain('Sürüm 2')
    expect(sms(w).value).toBe('Yeni')
    expect(w.find('[data-testid="error-retry"]').exists()).toBe(false)
  })

  it('yarıda kesilen akışta sürüm oluşmaz, düzenleyici açılmaz ve yeniden üretilebilir', async () => {
    routes({
      [GET]: () => detail(),
      [GENERATE]: () =>
        sseResponse([sseEvent('delta', { field: 'sms_text', text: 'Mer', replace: true })]),
    })
    const w = await mountView()
    await click(w, 'İçerik üret')

    expect(w.get('[data-testid="error-panel"]').text()).toContain('yarıda kesildi')
    expect(w.find('#sms_text').exists()).toBe(false)
    expect(w.find('[data-testid="version"]').exists()).toBe(false)
    expect(w.find('#approve-title').exists()).toBe(false)
    expect(w.find('[data-testid="error-retry"]').exists()).toBe(true)
  })

  it('onaylanmış kampanyada üretim 409 alırsa güncel (onaylı) durumu yükler', async () => {
    let approved = false
    routes({
      [GET]: () =>
        approved
          ? detail({
              status: 'approved',
              content: contentBody(),
              approved_content_version: 1,
              approved_at: '2026-11-18T11:00:00Z',
            })
          : detail(),
      [GENERATE]: () => {
        approved = true
        return errorResponse(409, 'CAMPAIGN_NOT_DRAFT')
      },
    })
    const w = await mountView()
    await click(w, 'İçerik üret')

    expect(w.get('[data-testid="approved"]').text()).toContain('Kampanya onaylandı')
    expect(button(w, 'İçerik üret')).toBeUndefined()
  })

  it('ekran kapanınca süren üretim iptal edilir', async () => {
    let signal: AbortSignal | null | undefined
    routes({
      [GET]: () => detail(),
      [GENERATE]: (init) => {
        signal = init?.signal
        return (sse = controlledSse(init?.signal)).response
      },
    })
    const w = await mountView()
    await click(w, 'İçerik üret')
    expect(signal?.aborted).toBe(false)

    w.unmount()
    await flushPromises()

    expect(signal?.aborted).toBe(true)
  })
})

describe('düzenleme ve kaydetme', () => {
  it('değişiklik onayı kapatır, kaydedince yeni sürüm olur ve onay yeniden açılır', async () => {
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [SAVE]: () =>
        jsonResponse(
          200,
          contentBody({ content_version: 2, source: 'human', sms_text: 'Düzenlendi' }),
        ),
    })
    const w = await mountView()
    expect(button(w, 'Yeni sürüm olarak kaydet')?.attributes('disabled')).toBeDefined() // değişiklik yok

    await w.get('#sms_text').setValue('Düzenlendi')
    expect(w.text()).toContain('Kaydedilmemiş değişiklik var.')
    expect(button(w, 'Onayla (sürüm 1)')?.attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('önce değişiklikleri kaydedin')

    await click(w, 'Yeni sürüm olarak kaydet')

    expect(w.get('[data-testid="version"]').text()).toBe('Sürüm 2 · Elle düzenlendi')
    expect(w.text()).not.toContain('Kaydedilmemiş değişiklik var.')
    expect(button(w, 'Onayla (sürüm 2)')).toBeDefined()
  })

  it('doğrulama hatasında mesajları alanların altında gösterir; yazınca temizlenir', async () => {
    routes({
      [GET]: () => detail({ content: contentBody() }),
      [SAVE]: () =>
        errorResponse(422, 'CONTENT_INVALID', [
          { code: 'UNKNOWN_VARIABLE', field: 'sms_text', text: 'city' },
          { code: 'EMPTY_FIELD', field: 'email_subject' },
        ]),
    })
    const w = await mountView()
    await w.get('#sms_text').setValue('Merhaba {{city}}')
    await click(w, 'Yeni sürüm olarak kaydet')

    expect(w.get('#sms-problems').text()).toContain('{{city}}')
    expect(w.get('#subject-problems').text()).toContain('boş olamaz')
    expect(w.get('[data-testid="version"]').text()).toContain('Sürüm 1') // sürüm artmadı

    await w.get('#sms_text').setValue('Merhaba')
    expect(w.get('#sms-problems').text()).toBe('')
  })

  it('üretmeden elle yazılan içerik kaydedilebilir', async () => {
    routes({
      [GET]: () => detail(),
      [SAVE]: () => jsonResponse(200, contentBody({ source: 'human', sms_text: 'Elle' })),
    })
    const w = await mountView()
    await click(w, 'İçeriği elle yaz')
    expect(sms(w).value).toBe('')
    expect(button(w, 'İçeriği elle yaz')).toBeUndefined()

    await w.get('#sms_text').setValue('Elle')
    await w.get('#email_subject').setValue('Konu')
    await w.get('#email_body').setValue('Gövde')
    await click(w, 'Yeni sürüm olarak kaydet')

    expect(w.get('[data-testid="version"]').text()).toBe('Sürüm 1 · Elle düzenlendi')
    expect(body(w).value).toBe(contentBody().email_body)
  })
})

describe('onay', () => {
  const draftWithContent = () => detail({ content: contentBody({ content_version: 1 }) })

  it('iki adımlı onaydan sonra kuyruğa alınan sayıları gösterir ve içeriği kilitler', async () => {
    routes({ [GET]: draftWithContent, [APPROVE]: () => jsonResponse(200, approvedBody) })
    const w = await mountView()

    await click(w, 'Onayla (sürüm 1)')
    expect(callsTo(APPROVE)).toHaveLength(0) // henüz istek yok
    expect(w.get('[role="alertdialog"]').text()).toContain('Onaydan sonra içerik değiştirilemez')
    await click(w, 'Onayla ve kuyruğa al')

    expect(JSON.parse(String(callsTo(APPROVE)[0]!.init?.body))).toEqual({ content_version: 1 })
    expect(w.get('[data-testid="approved"]').text()).toContain('Kampanya onaylandı: sürüm 1')
    expect(w.get('[data-testid="queued"]').text()).toContain('133 SMS ve 150 e-posta')
    expect(sms(w).readOnly).toBe(true)
    expect(button(w, 'İçerik üret')).toBeUndefined()
    expect(button(w, 'Yeniden üret')).toBeUndefined()
    expect(button(w, 'Yeni sürüm olarak kaydet')).toBeUndefined()
    expect(w.find('#approve-title').exists()).toBe(false)
    expect(w.text()).toContain('Onaylandı')
  })

  it('çift tıklama tek onay isteği yapar', async () => {
    let resolve!: (r: Response) => void
    routes({
      [GET]: draftWithContent,
      [APPROVE]: () => new Promise<Response>((res) => (resolve = res)),
    })
    const w = await mountView()
    await click(w, 'Onayla (sürüm 1)')
    const confirm = button(w, 'Onayla ve kuyruğa al')!
    void confirm.trigger('click')
    void confirm.trigger('click')
    await flushPromises()

    expect(callsTo(APPROVE)).toHaveLength(1)
    for (const b of w.findAll('button')) expect(b.attributes('disabled')).toBeDefined()
    resolve(jsonResponse(200, approvedBody))
    await flushPromises()
    expect(w.find('[data-testid="approved"]').exists()).toBe(true)
  })

  it('eski sürüm (409) uyarısında son sürümü yükler ve kendiliğinden onaylamaz', async () => {
    let reads = 0
    routes({
      [GET]: () =>
        detail({
          content:
            ++reads === 1
              ? contentBody({ content_version: 1 })
              : contentBody({
                  content_version: 2,
                  source: 'human',
                  sms_text: 'Başkasının düzenlediği',
                }),
        }),
      [APPROVE]: () => errorResponse(409, 'STALE_CONTENT_VERSION'),
    })
    const w = await mountView()
    await confirmApproval(w, 1)

    expect(w.get('[data-testid="error-panel"]').text()).toContain('güncel değil')
    expect(w.get('[data-testid="version"]').text()).toContain('Sürüm 2')
    expect(sms(w).value).toBe('Başkasının düzenlediği')
    expect(callsTo(APPROVE)).toHaveLength(1)
    expect(callsTo(GET)).toHaveLength(2)
    expect(w.find('[data-testid="approved"]').exists()).toBe(false)
  })

  it('render sınırı aşımında etkilenen alıcı sayısını ve örnek adı gösterir; kampanya taslak kalır', async () => {
    routes({
      [GET]: draftWithContent,
      [APPROVE]: () =>
        jsonResponse(422, {
          error: {
            code: 'RENDER_LIMIT_EXCEEDED',
            message: 'm',
            reasons: [
              { code: 'TOO_LONG', field: 'sms_text', count: 12, text: 'Zeynep' },
              { code: 'TOO_LONG', field: 'email_subject', count: 1, text: 'Değerli müşterimiz' },
            ],
          },
        }),
    })
    const w = await mountView()
    await confirmApproval(w, 1)

    const text = w.get('[data-testid="error-panel"]').text()
    expect(text).toContain('Metni kısaltın')
    expect(text).toContain('12 alıcı')
    expect(text).toContain('"Zeynep"')
    expect(text).toContain('1 alıcı')
    expect(w.find('[data-testid="approved"]').exists()).toBe(false)
    expect(button(w, 'Onayla (sürüm 1)')).toBeDefined() // düzeltip yeniden denenebilir
  })

  it.each([
    ['NO_ELIGIBLE_RECIPIENTS', 422, 'uygun alıcı yok'],
    ['UNFILTERED_SEGMENT', 422, 'Filtresiz kitle'],
    ['INTERNAL', 500, 'Beklenmeyen bir hata'],
  ])('%s hatasını Türkçe gösterir', async (code, status, text) => {
    routes({ [GET]: draftWithContent, [APPROVE]: () => errorResponse(status, code) })
    const w = await mountView()
    await confirmApproval(w, 1)

    expect(w.get('[data-testid="error-panel"]').text()).toContain(text)
    expect(w.find('[data-testid="approved"]').exists()).toBe(false)
  })

  it('başka yerde onaylanmış kampanyada (409) onaylı durumu yükler', async () => {
    let approved = false
    routes({
      [GET]: () =>
        approved
          ? detail({
              status: 'approved',
              content: contentBody({ content_version: 2 }),
              approved_content_version: 2,
              approved_at: '2026-11-18T11:00:00Z',
            })
          : detail({ content: contentBody({ content_version: 1 }) }),
      [APPROVE]: () => {
        approved = true
        return errorResponse(409, 'CAMPAIGN_ALREADY_APPROVED')
      },
    })
    const w = await mountView()
    await confirmApproval(w, 1)

    expect(w.get('[data-testid="approved"]').text()).toContain('sürüm 2')
    expect(button(w, 'İçerik üret')).toBeUndefined()
  })

  it('onaylı kampanya yüklenince salt okunur gösterilir; hiçbir işlem düğmesi yoktur', async () => {
    routes({
      [GET]: () =>
        detail({
          status: 'sending',
          content: contentBody({ content_version: 2 }),
          approved_content_version: 2,
          approved_at: '2026-11-18T11:00:00Z',
        }),
    })
    const w = await mountView()

    expect(w.get('[data-testid="approved"]').text()).toContain('Kampanya onaylandı: sürüm 2')
    expect(w.find('[data-testid="queued"]').exists()).toBe(false) // sayılar yalnızca onay yanıtında
    expect(sms(w).readOnly).toBe(true)
    expect(w.findAll('button')).toHaveLength(0)
  })

  // Gönderim takibine geçiş: düğme değil bağlantıdır (işlem düğmesi sayılmaz).
  describe('gönderimi izleme bağlantısı', () => {
    const deliveryLink = (w: VueWrapper) =>
      w.findAllComponents(RouterLinkStub).find((l) => l.props('to') === `/campaigns/${ID}/delivery`)

    it('onay sonrası gönderim takibi ekranına bağlantı verir', async () => {
      routes({
        [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
        [APPROVE]: () => jsonResponse(200, approvedBody),
      })
      const w = await mountView()
      expect(deliveryLink(w)).toBeUndefined() // taslakta gönderim takibi yok

      await confirmApproval(w, 1)

      const link = deliveryLink(w)
      expect(link?.text()).toBe('İleri: Gönderimi izle')
      expect(w.findAll('button')).toHaveLength(0) // bağlantı, işlem düğmesi değil
    })

    it('onaylı kampanya yeniden yüklenince de bağlantı vardır', async () => {
      routes({
        [GET]: () =>
          detail({
            status: 'sending',
            content: contentBody({ content_version: 2 }),
            approved_content_version: 2,
            approved_at: '2026-11-18T11:00:00Z',
          }),
      })
      const w = await mountView()

      expect(deliveryLink(w)).toBeDefined()
      expect(w.find('[data-testid="delivery-link"]').exists()).toBe(true)
    })

    it('taslak kampanyada bağlantı yoktur', async () => {
      routes({ [GET]: () => detail() })
      const w = await mountView()

      expect(deliveryLink(w)).toBeUndefined()
      expect(w.find('[data-testid="delivery-link"]').exists()).toBe(false)
    })
  })
})

// Aynı ekran bileşeni /campaigns/A'dan /campaigns/B'ye geçince yeniden kullanılır
// (route parametresi değişir). Süren işlemlerin durumu B'ye sızmamalı, B'nin
// düğmeleri kilitli kalmamalı ve A'dan geç gelen yanıtlar B'yi değiştirmemelidir.
describe('başka kampanyaya geçiş (süren işlemler)', () => {
  const ID_B = '11111111-2222-3333-4444-555555555555'
  const GET_B = `GET /api/campaigns/${ID_B}`
  const GENERATE_B = `POST /api/campaigns/${ID_B}/content`
  const bDetail = (extra: Record<string, unknown> = {}) =>
    jsonResponse(200, { ...campaignBody({ id: ID_B, brief: 'B kampanyası' }), ...extra })

  async function switchToB(w: VueWrapper) {
    await w.setProps({ id: ID_B })
    await flushPromises()
  }

  it('üretim sürerken geçilirse: A iptal edilir, B kilitli kalmaz ve B üretebilir', async () => {
    let sse!: ControlledSse
    let signalA: AbortSignal | null | undefined
    routes({
      [GET]: () => detail(),
      [GENERATE]: (init) => {
        signalA = init?.signal
        return (sse = controlledSse(init?.signal)).response
      },
      [GET_B]: () => bDetail(),
      [GENERATE_B]: () =>
        sseResponse([sseEvent('validated', contentBody({ sms_text: 'B içeriği' }))]),
    })
    const w = await mountView()
    await click(w, 'İçerik üret')
    sse.push(sseEvent('delta', { field: 'sms_text', text: 'A metni', replace: true }))
    await vi.waitFor(() => expect(sms(w).value).toBe('A metni'))

    await switchToB(w)

    expect(signalA?.aborted).toBe(true)
    expect(w.text()).toContain('B kampanyası')
    expect(w.find('[data-testid="error-panel"]').exists()).toBe(false) // iptal hata olarak gösterilmez
    expect(w.find('#sms_text').exists()).toBe(false) // A'nın geçici metni B'de yok
    const generate = button(w, 'İçerik üret')
    expect(button(w, 'Üretiliyor…')).toBeUndefined()
    expect(generate?.attributes('disabled')).toBeUndefined()
    expect(w.text()).not.toContain('Gördüğünüz metin geçicidir')

    // A'nın akışından geç gelen olay B'yi değiştirmez.
    try {
      sse.push(sseEvent('delta', { field: 'sms_text', text: 'GEÇ A', replace: true }))
    } catch {
      /* akış iptalle zaten kapandı */
    }
    await flushPromises()
    expect(w.find('#sms_text').exists()).toBe(false)

    await click(w, 'İçerik üret')
    expect(sms(w).value).toBe('B içeriği')
    expect(callsTo(GENERATE_B)).toHaveLength(1)
  })

  it("kaydetme sürerken geçilirse: B kilitli kalmaz, A'nın geç yanıtı B'yi değiştirmez", async () => {
    let resolvePut!: (r: Response) => void
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [SAVE]: () => new Promise<Response>((res) => (resolvePut = res)),
      [GET_B]: () => bDetail({ content: contentBody({ content_version: 7, sms_text: 'B SMS' }) }),
    })
    const w = await mountView()
    await w.get('#sms_text').setValue('A düzenlemesi')
    await click(w, 'Yeni sürüm olarak kaydet')
    expect(button(w, 'Kaydediliyor…')).toBeDefined()

    await switchToB(w)

    expect(button(w, 'Kaydediliyor…')).toBeUndefined()
    expect(w.get('[data-testid="version"]').text()).toContain('Sürüm 7')
    expect(sms(w).value).toBe('B SMS')
    expect(sms(w).readOnly).toBe(false)
    expect(button(w, 'Onayla (sürüm 7)')?.attributes('disabled')).toBeUndefined()

    // A'nın PUT'u şimdi biterse B'nin durumu değişmemeli.
    resolvePut(
      jsonResponse(
        200,
        contentBody({ content_version: 2, source: 'human', sms_text: 'A düzenlemesi' }),
      ),
    )
    await flushPromises()
    expect(w.get('[data-testid="version"]').text()).toContain('Sürüm 7')
    expect(sms(w).value).toBe('B SMS')
  })

  it("onay sürerken geçilirse: B kilitli kalmaz, A'nın geç onayı B'yi onaylı göstermez", async () => {
    let resolveApprove!: (r: Response) => void
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 1 }) }),
      [APPROVE]: () => new Promise<Response>((res) => (resolveApprove = res)),
      [GET_B]: () => bDetail({ content: contentBody({ content_version: 3 }) }),
    })
    const w = await mountView()
    await confirmApproval(w, 1)
    for (const b of w.findAll('button')) expect(b.attributes('disabled')).toBeDefined()

    await switchToB(w)

    expect(w.text()).toContain('B kampanyası')
    expect(button(w, 'Onayla (sürüm 3)')?.attributes('disabled')).toBeUndefined()
    expect(button(w, 'Yeniden üret')?.attributes('disabled')).toBeUndefined()

    resolveApprove(jsonResponse(200, approvedBody))
    await flushPromises()
    expect(w.find('[data-testid="approved"]').exists()).toBe(false)
    expect(w.text()).toContain('Taslak')
    expect(button(w, 'Onayla (sürüm 3)')).toBeDefined()
  })

  it("A'da kalan hata, kısmi metin ve düzenleme B'ye taşınmaz", async () => {
    routes({
      [GET]: () => detail(),
      [GENERATE]: () =>
        sseResponse([
          sseEvent('error', {
            code: 'CONTENT_INVALID',
            message: 'm',
            reasons: [{ code: 'TOO_LONG', field: 'sms_text' }],
            partial: { sms_text: 'x'.repeat(200), email_subject: 'Konu', email_body: 'Gövde' },
          }),
        ]),
      [GET_B]: () => bDetail(),
    })
    const w = await mountView()
    await click(w, 'İçerik üret')
    expect(w.find('[data-testid="error-panel"]').exists()).toBe(true)
    expect(w.text()).toContain('henüz kaydedilmedi')

    await switchToB(w)

    expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
    expect(w.find('#sms_text').exists()).toBe(false)
    expect(w.text()).not.toContain('henüz kaydedilmedi')
    expect(w.text()).not.toContain('Kaydedilmemiş değişiklik')
  })

  it("B yüklenirken A'nın içeriği görünmez; B yüklenemezse eski içerik ve düğmeler kalmaz", async () => {
    routes({
      [GET]: () => detail({ content: contentBody({ content_version: 4, sms_text: 'A SMS' }) }),
      [GET_B]: () => errorResponse(404, 'CAMPAIGN_NOT_FOUND'),
    })
    const w = await mountView()
    expect(sms(w).value).toBe('A SMS')

    await switchToB(w)

    expect(w.get('[data-testid="error-panel"]').text()).toContain('Kampanya bulunamadı.')
    expect(w.find('#sms_text').exists()).toBe(false)
    // Eski içeriğin işlem düğmeleri kalmaz; yalnızca (kapalı) gezinti düğmesi olabilir.
    const actions = w
      .findAll('button')
      .filter((b) => !b.attributes('data-testid')?.startsWith('nav-'))
    expect(actions).toHaveLength(0)
  })
})
