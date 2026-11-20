import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, type PropType } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import RecipientsSheet from '../RecipientsSheet.vue'
import { deferred, errorResponse, jsonResponse } from '@/__tests__/helpers'
import { recipientBody, recipientPageBody, recipients } from '@/__tests__/recipient-helpers'

const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'
const fetchMock = vi.fn<typeof fetch>()

// Bu dosya çekmecenin MANTIĞINI sınar (istekler, süzgeçler, durumlar, tazeleme). Gerçek shadcn
// `Select` jsdom'da açılır liste ve işaretçi olayları gerektirdiği için aynı arayüzlü yerel bir
// seçiciyle değiştirilir; gerçek bileşen `FilterSelect.spec.ts`'te sınanır.
const FilterSelectStub = defineComponent({
  name: 'FilterSelect',
  inheritAttrs: false,
  props: {
    id: { type: String, required: true },
    modelValue: { type: String, required: true },
    options: { type: Array as PropType<{ value: string; label: string }[]>, required: true },
    allLabel: { type: String, required: true },
  },
  emits: ['update:modelValue'],
  setup(props, { emit, attrs }) {
    return () =>
      h(
        'select',
        {
          ...attrs,
          id: props.id,
          value: props.modelValue,
          onChange: (e: Event) => emit('update:modelValue', (e.target as HTMLSelectElement).value),
        },
        [
          h('option', { value: '' }, props.allLabel),
          ...props.options.map((o) => h('option', { value: o.value }, o.label)),
        ],
      )
  },
})

let wrapper: VueWrapper | null = null

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  // Yalnızca zamanlayıcılar sahte: flushPromises ve söz zincirleri gerçek kalır.
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

/** Sayfalı yanıt verir: toplam `all`, her sayfa 10 satır. */
function serve(all = 25) {
  fetchMock.mockImplementation((input) => {
    const url = new URL(String(input), 'http://x')
    const page = Number(url.searchParams.get('page'))
    const first = (page - 1) * 10
    const count = Math.max(0, Math.min(10, all - first))
    return Promise.resolve(
      jsonResponse(
        200,
        recipientPageBody({ page, total: all, items: recipients(count, first + 1) }),
      ),
    )
  })
}

const calls = () => fetchMock.mock.calls.map(([u]) => new URL(String(u), 'http://x'))
const lastParams = () => calls()[calls().length - 1]!.searchParams

async function mountSheet(props: { open?: boolean; live?: boolean; campaignId?: string } = {}) {
  wrapper = mount(RecipientsSheet, {
    attachTo: document.body,
    props: {
      campaignId: props.campaignId ?? ID,
      live: props.live ?? false,
      open: props.open ?? true,
      'onUpdate:open': (value: boolean) => void wrapper?.setProps({ open: value }),
    },
    global: { stubs: { FilterSelect: FilterSelectStub } },
  })
  await flushPromises()
  return wrapper
}

const q = <T extends Element = HTMLElement>(testid: string) =>
  document.body.querySelector<T>(`[data-testid="${testid}"]`)
const qa = (testid: string) =>
  Array.from(document.body.querySelectorAll(`[data-testid="${testid}"]`))
const texts = (testid: string) =>
  qa(testid).map((el) => el.textContent?.replace(/\s+/g, ' ').trim())

async function type(testid: string, text: string) {
  const input = q<HTMLInputElement>(testid)!
  input.value = text
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}
async function choose(testid: string, value: string) {
  const select = q<HTMLSelectElement>(testid)!
  select.value = value
  select.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

describe('RecipientsSheet', () => {
  describe('açılış', () => {
    it('kapalıyken hiçbir istek yapmaz ve içerik çizmez', async () => {
      serve()
      await mountSheet({ open: false })

      expect(fetchMock).not.toHaveBeenCalled()
      expect(q('recipients-sheet')).toBeNull()
    })

    it('açılınca süzgeçsiz ilk sayfayı ister ve satırları listeler', async () => {
      serve()
      const w = await mountSheet({ open: false })

      await w.setProps({ open: true })
      await flushPromises()

      expect(calls()).toHaveLength(1)
      expect(calls()[0]!.pathname).toBe(`/api/campaigns/${ID}/recipients`)
      expect(Object.fromEntries(lastParams())).toEqual({ page: '1', limit: '10' })
      expect(qa('recipient-row')).toHaveLength(10)
      expect(q('recipients-total')!.textContent?.trim()).toBe('25 alıcı')
    })

    it('başlık, açıklama ve Türkçe kapatma düğmesi vardır (ekran okuyucu için ad)', async () => {
      serve()
      await mountSheet()

      expect(document.body.querySelector('[role="dialog"]')).not.toBeNull()
      expect(document.body.querySelector('[data-slot="sheet-title"]')!.textContent).toBe('Alıcılar')
      expect(q('recipients-close')!.getAttribute('aria-label')).toBe('Alıcılar çekmecesini kapat')
    })

    it('yanıt gelene kadar iskelet gösterir', async () => {
      fetchMock.mockReturnValue(deferred<Response>().promise)
      await mountSheet()

      expect(q('recipients-loading')).not.toBeNull()
      expect(qa('recipient-row')).toHaveLength(0)
    })
  })

  describe('satırlar', () => {
    it('kim, hangi kanal ve adres, ne oldu: ad, adres, kanal, durum ve gönderim zamanı', async () => {
      fetchMock.mockResolvedValue(
        jsonResponse(
          200,
          recipientPageBody({
            items: [
              recipientBody({
                id: 1,
                first_name: 'Ayşe',
                address: '+905551112233',
                channel: 'sms',
              }),
              recipientBody({
                id: 2,
                first_name: null,
                address: 'ad-yok@example.test',
                channel: 'email',
                status: 'failed',
                sent_at: null,
                failure_reason: 'rejected',
              }),
              recipientBody({
                id: 3,
                first_name: 'Can',
                status: 'queued',
                attempts: 2,
                sent_at: null,
              }),
            ],
          }),
        ),
      )
      await mountSheet()

      expect(texts('recipient-name')).toEqual(['Ayşe', 'Ad yok', 'Can'])
      expect(texts('recipient-address')[0]).toBe('+905551112233')
      expect(texts('recipient-channel')).toEqual(['SMS', 'E-posta', 'SMS'])
      expect(texts('recipient-status')[0]).toMatch(/^Gönderildi\s*20 Kas \d{2}:\d{2}$/)
      expect(texts('recipient-status')[1]).toBe('BaşarısızSağlayıcı reddetti')
      expect(texts('recipient-status')[2]).toBe('Yeniden denenecek')
      expect(texts('recipient-sent-at')).toHaveLength(1) // yalnızca gönderilenin zamanı var
      expect(texts('recipient-failure')).toEqual(['Sağlayıcı reddetti'])
      expect(document.body.querySelectorAll('[data-missing="true"]')).toHaveLength(1)
    })

    it('veriyi HTML olarak yorumlamaz', async () => {
      fetchMock.mockResolvedValue(
        jsonResponse(
          200,
          recipientPageBody({
            items: [
              recipientBody({ first_name: '<img src=x onerror=alert(1)>', address: '<b>x</b>' }),
            ],
          }),
        ),
      )
      await mountSheet()

      expect(q('recipients-body')!.querySelector('img')).toBeNull()
      expect(q('recipients-body')!.querySelector('b')).toBeNull()
      expect(texts('recipient-name')[0]).toBe('<img src=x onerror=alert(1)>')
    })
  })

  describe('süzgeçler', () => {
    it('arama yazarken 300 ms bekler, sonra ilk sayfayı süzgeçle ister (aralıktaki yazımlar tek istek)', async () => {
      serve()
      await mountSheet()
      fetchMock.mockClear()

      await type('recipients-search', 'a')
      await type('recipients-search', 'ay')
      await type('recipients-search', '  ayşe ')
      await vi.advanceTimersByTimeAsync(299)
      expect(fetchMock).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(1)
      await flushPromises()

      expect(calls()).toHaveLength(1)
      expect(Object.fromEntries(lastParams())).toEqual({ page: '1', limit: '10', q: 'ayşe' })
    })

    it('Enter ile arama beklemeden çalışır', async () => {
      serve()
      await mountSheet()
      fetchMock.mockClear()

      await type('recipients-search', 'can')
      q('recipients-filters')!.dispatchEvent(new Event('submit', { cancelable: true }))
      await flushPromises()

      expect(lastParams().get('q')).toBe('can')
    })

    it('kanal ve durum seçilince ilk sayfa o süzgeçlerle istenir; "tümü"ne dönünce süzgeç kalkar', async () => {
      serve()
      await mountSheet()

      await choose('recipients-channel', 'email')
      await choose('recipients-status', 'failed')
      expect(Object.fromEntries(lastParams())).toEqual({
        page: '1',
        limit: '10',
        channel: 'email',
        status: 'failed',
      })

      await choose('recipients-channel', '')
      expect(lastParams().has('channel')).toBe(false)
      expect(lastParams().get('status')).toBe('failed')
    })

    it('süzgeç ikinci sayfadayken değişirse ilk sayfaya döner', async () => {
      serve()
      await mountSheet()
      await document.body.querySelector<HTMLElement>('[data-slot="pagination-next"]')!.click()
      await flushPromises()
      expect(lastParams().get('page')).toBe('2')

      await choose('recipients-status', 'sent')

      expect(lastParams().get('page')).toBe('1')
    })

    it('"Filtreleri temizle" bütün süzgeçleri kaldırır ve etkin süzgeç sayısını gösterir', async () => {
      serve()
      await mountSheet()
      await choose('recipients-channel', 'sms')
      await choose('recipients-status', 'sent')
      expect(q('recipients-clear')!.textContent).toContain('2')

      q('recipients-clear')!.click()
      await flushPromises()

      expect(Object.fromEntries(lastParams())).toEqual({ page: '1', limit: '10' })
      expect(q('recipients-clear')).toBeNull()
    })

    it('süzgece uyan alıcı yoksa boş durum ve temizleme düğmesi gösterir', async () => {
      // Her istek için yeni Response: gövde yalnızca bir kez okunabilir.
      fetchMock.mockImplementation(() =>
        Promise.resolve(jsonResponse(200, recipientPageBody({ items: [], total: 0 }))),
      )
      await mountSheet()
      expect(q('recipients-empty')!.textContent).toContain('Henüz alıcı yok')

      await choose('recipients-status', 'failed')

      expect(q('recipients-empty')!.textContent).toContain('Filtrelere uyan alıcı yok')
      expect(q('recipients-empty')!.textContent).toContain('Filtreleri temizle')
    })
  })

  describe('sayfalama', () => {
    it('Sonraki sayfa ikinci sayfayı ister ve özeti günceller', async () => {
      serve()
      await mountSheet()

      document.body.querySelector<HTMLElement>('[data-slot="pagination-next"]')!.click()
      await flushPromises()

      expect(lastParams().get('page')).toBe('2')
      expect(texts('recipient-name')[0]).toBe('Kişi 11')
      expect(document.body.textContent).toContain('11–20 / 25 kayıt · Sayfa 2 / 3')
    })

    it('liste kısalınca ve sayfa boş kalınca son sayfaya gider', async () => {
      serve(25)
      await mountSheet()
      // Sunucu artık yalnızca 5 kayıt olduğunu söylüyor: ikinci sayfa boş, son sayfa 1.
      fetchMock.mockImplementation((input) => {
        const page = Number(new URL(String(input), 'http://x').searchParams.get('page'))
        return Promise.resolve(
          jsonResponse(
            200,
            page > 1
              ? recipientPageBody({ page, total: 5, items: [] })
              : recipientPageBody({ page, total: 5, items: recipients(5) }),
          ),
        )
      })
      fetchMock.mockClear()

      document.body.querySelector<HTMLElement>('[data-slot="pagination-next"]')!.click()
      await flushPromises()

      expect(calls().map((u) => u.searchParams.get('page'))).toEqual(['2', '1'])
      expect(qa('recipient-row')).toHaveLength(5)
      expect(q('recipients-total')!.textContent?.trim()).toBe('5 alıcı')
    })
  })

  describe('hata', () => {
    it('yükleme hatasında hata paneli gösterir ve "Yeniden dene" aynı sayfayı ister', async () => {
      fetchMock.mockResolvedValueOnce(errorResponse(500, 'INTERNAL'))
      await mountSheet()
      expect(document.body.querySelector('[data-testid="error-panel"]')).not.toBeNull()
      expect(qa('recipient-row')).toHaveLength(0)

      serve()
      q('error-retry')!.click()
      await flushPromises()

      expect(document.body.querySelector('[data-testid="error-panel"]')).toBeNull()
      expect(qa('recipient-row')).toHaveLength(10)
    })
  })

  describe('canlı tazeleme', () => {
    it('gönderim sürerken 5 saniyede bir aynı sayfayı sessizce tazeler (iskelet çıkmaz)', async () => {
      serve()
      await mountSheet({ live: true })
      fetchMock.mockClear()

      await vi.advanceTimersByTimeAsync(5_000)
      await nextTick()

      expect(calls()).toHaveLength(1)
      expect(lastParams().get('page')).toBe('1')
      expect(q('recipients-loading')).toBeNull()
      expect(qa('recipient-row')).toHaveLength(10)
    })

    it('tazeleme başarısız olursa ekrandaki liste ve hata yokluğu korunur', async () => {
      serve()
      await mountSheet({ live: true })
      fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))

      await vi.advanceTimersByTimeAsync(5_000)
      await flushPromises()

      expect(qa('recipient-row')).toHaveLength(10)
      expect(document.body.querySelector('[data-testid="error-panel"]')).toBeNull()
    })

    it('gönderim bitmişse (live=false) tazelemez', async () => {
      serve()
      await mountSheet({ live: false })
      fetchMock.mockClear()

      await vi.advanceTimersByTimeAsync(30_000)

      expect(fetchMock).not.toHaveBeenCalled()
    })

    it('kapatılınca tazeleme durur', async () => {
      serve()
      const w = await mountSheet({ live: true })
      await w.setProps({ open: false })
      await flushPromises()
      fetchMock.mockClear()

      await vi.advanceTimersByTimeAsync(30_000)

      expect(fetchMock).not.toHaveBeenCalled()
    })
  })

  describe('kampanya ve yaşam döngüsü', () => {
    it('başka kampanyaya geçilince eski satırlar kalkar, süzgeçler sıfırlanır ve yeni kampanya okunur', async () => {
      serve()
      const w = await mountSheet()
      await choose('recipients-status', 'sent')
      fetchMock.mockClear()

      await w.setProps({ campaignId: 'diger-kampanya' })
      await flushPromises()

      expect(calls()).toHaveLength(1)
      expect(calls()[0]!.pathname).toBe('/api/campaigns/diger-kampanya/recipients')
      expect(Object.fromEntries(lastParams())).toEqual({ page: '1', limit: '10' })
    })

    it('geç gelen eski yanıt yeni sonucu ezmez', async () => {
      const slow = deferred<Response>()
      fetchMock.mockReturnValueOnce(slow.promise)
      await mountSheet()
      serve(3)

      await choose('recipients-status', 'sent') // yeni istek, eskisini iptal eder
      slow.resolve(jsonResponse(200, recipientPageBody({ total: 99, items: recipients(10) })))
      await flushPromises()

      expect(q('recipients-total')!.textContent?.trim()).toBe('3 alıcı')
    })

    it('kapatılınca sürmekte olan istek iptal edilir', async () => {
      let signal: AbortSignal | null | undefined
      fetchMock.mockImplementation((_url, init) => {
        signal = init?.signal
        return deferred<Response>().promise
      })
      const w = await mountSheet()

      await w.setProps({ open: false })

      expect(signal?.aborted).toBe(true)
    })
  })
})
