import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, type PropType } from 'vue'
import { flushPromises, mount as mountComponent, type VueWrapper } from '@vue/test-utils'
import AudienceDirectory from '../AudienceDirectory.vue'
import {
  audiencePageBody,
  deferred,
  errorResponse,
  jsonResponse,
  memberBody,
  members,
} from '@/__tests__/helpers'

const fetchMock = vi.fn<typeof fetch>()

// Bu dosya dizinin MANTIĞINI sınar (süzgeç birleşimi, istekler, durumlar). Gerçek shadcn `Select`
// jsdom'da açılır liste, portal ve işaretçi olayları gerektirdiği için burada aynı arayüzlü (id,
// v-model, seçenekler, "tümü" etiketi) yerel bir seçiciyle değiştirilir; gerçek bileşen
// `FilterSelect.spec.ts`'te sınanır.
const FilterSelectStub = defineComponent({
  name: 'FilterSelect',
  inheritAttrs: false,
  props: {
    id: { type: String, required: true },
    modelValue: { type: String, required: true },
    options: { type: Array as PropType<{ value: string; label: string }[]>, required: true },
    allLabel: { type: String, required: true },
    disabled: Boolean,
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
          disabled: props.disabled,
          onChange: (e: Event) => emit('update:modelValue', (e.target as HTMLSelectElement).value),
        },
        [
          h('option', { value: '' }, props.allLabel),
          ...props.options.map((o) => h('option', { value: o.value }, o.label)),
        ],
      )
  },
})

function mount(component: typeof AudienceDirectory, options: { attachTo?: HTMLElement } = {}) {
  return mountComponent(component, {
    ...options,
    global: { stubs: { FilterSelect: FilterSelectStub } },
  })
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  // Yalnızca zamanlayıcılar sahte: flushPromises ve söz zincirleri gerçek kalır.
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const CITIES = ['Ankara', 'Adana', 'İzmir']

/** Adrese göre yanıtlar: şehirler ve (varsayılan 25 kişilik) sayfalı dizin. */
function serve(total: number | ((params: URLSearchParams) => number) = 25) {
  fetchMock.mockImplementation((input) => {
    const url = new URL(String(input), 'http://x')
    if (url.pathname === '/api/audience/cities') {
      return Promise.resolve(jsonResponse(200, { cities: CITIES }))
    }
    const page = Number(url.searchParams.get('page'))
    const all = typeof total === 'function' ? total(url.searchParams) : total
    const first = (page - 1) * 10
    const count = Math.max(0, Math.min(10, all - first))
    return Promise.resolve(
      jsonResponse(200, audiencePageBody({ page, total: all, items: members(count, first + 1) })),
    )
  })
}

const directoryCalls = () =>
  fetchMock.mock.calls
    .map(([url]) => new URL(String(url), 'http://x'))
    .filter((u) => u.pathname === '/api/audience')
const lastParams = () => directoryCalls()[directoryCalls().length - 1]!.searchParams
const rows = (w: VueWrapper) => w.findAll('[data-testid="member-row"]')
const byId = (w: VueWrapper, id: string) => w.get(`[data-testid="${id}"]`)

async function mountDirectory() {
  const w = mount(AudienceDirectory)
  await flushPromises()
  return w
}

async function typeSearch(w: VueWrapper, text: string) {
  await byId(w, 'directory-search').setValue(text)
}

describe('AudienceDirectory', () => {
  describe('ilk yükleme', () => {
    it('süzgeçsiz ilk sayfayı ve şehir seçeneklerini ister', async () => {
      serve()
      const w = await mountDirectory()

      expect(directoryCalls()).toHaveLength(1)
      expect(Object.fromEntries(lastParams())).toEqual({ page: '1', limit: '10' })
      expect(fetchMock.mock.calls.map(([u]) => String(u))).toContain('/api/audience/cities')
      expect(rows(w)).toHaveLength(10)
      expect(byId(w, 'directory-total').text()).toBe('25 kişi')
    })

    it('yanıt gelene kadar iskelet gösterir', async () => {
      fetchMock.mockReturnValue(deferred<Response>().promise)
      const w = mount(AudienceDirectory)
      await flushPromises()

      const loading = byId(w, 'directory-loading')
      expect(loading.attributes('role')).toBe('status')
      expect(loading.text()).toContain('yükleniyor')
      expect(rows(w)).toHaveLength(0)
    })

    it('şehirler alfabetik (Türkçe) sıralı seçenek olur; ilk seçenek "Tüm şehirler"dir', async () => {
      serve()
      const w = await mountDirectory()

      const options = byId(w, 'directory-city')
        .findAll('option')
        .map((o) => o.text())
      expect(options).toEqual(['Tüm şehirler', 'Adana', 'Ankara', 'İzmir'])
    })

    it('uygunluk seçenekleri Tümü, Uygun, İzin yok ve Adres yok', async () => {
      serve()
      const w = await mountDirectory()

      for (const id of ['directory-sms', 'directory-email']) {
        expect(
          byId(w, id)
            .findAll('option')
            .map((o) => o.text()),
        ).toEqual(['Tümü', 'Uygun', 'İzin yok', 'Adres yok'])
      }
    })

    it('her süzgecin görünür bir etiketi vardır', async () => {
      serve()
      const w = await mountDirectory()

      const labels = w.findAll('label').map((l) => l.text())
      expect(labels).toEqual(['Kişi ara', 'Şehir', 'SMS uygunluğu', 'E-posta uygunluğu'])
      expect(byId(w, 'directory-search').attributes('id')).toBe('directory-search')
    })
  })

  describe('süzgeçler', () => {
    it('şehir seçince ilk sayfa o şehirle istenir', async () => {
      serve()
      const w = await mountDirectory()

      await byId(w, 'directory-city').setValue('Adana')
      await flushPromises()

      expect(directoryCalls()).toHaveLength(2)
      expect(lastParams().get('city')).toBe('Adana')
      expect(lastParams().get('page')).toBe('1')
    })

    it('SMS ve e-posta uygunluğunu ayrı ayrı süzer ve birleştirir', async () => {
      serve()
      const w = await mountDirectory()

      await byId(w, 'directory-sms').setValue('eligible')
      await flushPromises()
      expect(lastParams().get('sms')).toBe('eligible')
      expect(lastParams().has('email')).toBe(false)

      await byId(w, 'directory-email').setValue('no_consent')
      await flushPromises()
      expect(lastParams().get('sms')).toBe('eligible')
      expect(lastParams().get('email')).toBe('no_consent')
    })

    it('"Tümü"ne dönmek süzgeci kaldırır', async () => {
      serve()
      const w = await mountDirectory()
      await byId(w, 'directory-sms').setValue('no_address')
      await flushPromises()

      await byId(w, 'directory-sms').setValue('')
      await flushPromises()

      expect(lastParams().has('sms')).toBe(false)
    })

    it("süzgeç değişince sayfa 1'e döner", async () => {
      serve()
      const w = await mountDirectory()
      await w.get('[aria-label="Sayfa 3"]').trigger('click')
      await flushPromises()
      expect(lastParams().get('page')).toBe('3')

      await byId(w, 'directory-city').setValue('İzmir')
      await flushPromises()

      expect(lastParams().get('page')).toBe('1')
      expect(lastParams().get('city')).toBe('İzmir')
    })

    it('süzgeç etkin olunca "Filtreleri temizle" etkin sayıyı rozetle gösterir', async () => {
      serve()
      const w = await mountDirectory()
      expect(w.find('[data-testid="directory-clear"]').exists()).toBe(false)

      await byId(w, 'directory-city').setValue('Adana')
      await byId(w, 'directory-sms').setValue('eligible')
      await flushPromises()

      expect(byId(w, 'directory-clear').text()).toContain('Filtreleri temizle')
      expect(byId(w, 'directory-clear').text()).toContain('2')
    })

    it('"Filtreleri temizle" hepsini sıfırlar ve tek istek yapar', async () => {
      serve()
      const w = await mountDirectory()
      await byId(w, 'directory-city').setValue('Adana')
      await byId(w, 'directory-sms').setValue('eligible')
      await typeSearch(w, 'ayşe')
      await vi.advanceTimersByTimeAsync(300)
      await flushPromises()
      const before = directoryCalls().length

      await byId(w, 'directory-clear').trigger('click')
      await flushPromises()

      expect(directoryCalls()).toHaveLength(before + 1)
      expect(Object.fromEntries(lastParams())).toEqual({ page: '1', limit: '10' })
      expect((byId(w, 'directory-search').element as HTMLInputElement).value).toBe('')
      expect((byId(w, 'directory-city').element as HTMLSelectElement).value).toBe('')
      expect(w.find('[data-testid="directory-clear"]').exists()).toBe(false)
    })

    it('şehir listesi yüklenemezse şehir seçici kapanır ve söyler; diğer süzgeçler çalışır', async () => {
      fetchMock.mockImplementation((input) => {
        const url = String(input)
        return Promise.resolve(
          url.includes('/cities')
            ? errorResponse(500, 'INTERNAL')
            : jsonResponse(200, audiencePageBody({ items: members(3), total: 3 })),
        )
      })
      const w = await mountDirectory()

      const city = byId(w, 'directory-city')
      expect(city.attributes('disabled')).toBeDefined()
      expect(city.findAll('option')[0]!.text()).toBe('Şehirler yüklenemedi')
      await byId(w, 'directory-sms').setValue('eligible')
      await flushPromises()
      expect(lastParams().get('sms')).toBe('eligible')
    })
  })

  describe('arama', () => {
    it('yazarken 300 ms bekler ve tek istek yapar', async () => {
      serve()
      const w = await mountDirectory()

      await typeSearch(w, 'a')
      await vi.advanceTimersByTimeAsync(200)
      await typeSearch(w, 'ay')
      await vi.advanceTimersByTimeAsync(200)
      await typeSearch(w, 'ayş')
      expect(directoryCalls()).toHaveLength(1) // henüz istek yok
      await vi.advanceTimersByTimeAsync(300)
      await flushPromises()

      expect(directoryCalls()).toHaveLength(2)
      expect(lastParams().get('search')).toBe('ayş')
      expect(lastParams().get('page')).toBe('1')
    })

    it('Enter (form gönderimi) beklemeden arar', async () => {
      serve()
      const w = await mountDirectory()
      await typeSearch(w, 'kaya')

      await byId(w, 'directory-filters').trigger('submit')
      await flushPromises()

      expect(lastParams().get('search')).toBe('kaya')
      // Bekleyen zamanlayıcı ikinci istek yapmaz.
      await vi.advanceTimersByTimeAsync(500)
      await flushPromises()
      expect(directoryCalls()).toHaveLength(2)
    })

    it('"Ara" düğmesi de beklemeden arar', async () => {
      serve()
      // Form belgeye bağlı olmalı: jsdom bağlı olmayan formu göndermez (tarayıcı gönderir).
      const w = mount(AudienceDirectory, { attachTo: document.body })
      await flushPromises()
      await typeSearch(w, 'kaya')

      expect(w.get('button[type="submit"]').text()).toBe('Ara')
      await w.get('button[type="submit"]').trigger('click')
      await flushPromises()

      expect(directoryCalls()).toHaveLength(2) // 300 ms beklenmeden
      expect(lastParams().get('search')).toBe('kaya')
      w.unmount()
    })

    it('metni kırpar: yalnızca boşluk yazmak arama sayılmaz ve istek yapmaz', async () => {
      serve()
      const w = await mountDirectory()

      await typeSearch(w, '    ')
      await vi.advanceTimersByTimeAsync(400)
      await flushPromises()

      expect(directoryCalls()).toHaveLength(1)
    })

    it('"x" düğmesi aramayı temizler ve listeyi süzgeçsiz yeniler', async () => {
      serve()
      const w = await mountDirectory()
      await typeSearch(w, 'kaya')
      await byId(w, 'directory-filters').trigger('submit')
      await flushPromises()
      expect(lastParams().get('search')).toBe('kaya')

      await byId(w, 'directory-search-clear').trigger('click')
      await flushPromises()

      expect(lastParams().has('search')).toBe(false)
      expect((byId(w, 'directory-search').element as HTMLInputElement).value).toBe('')
      expect(w.find('[data-testid="directory-search-clear"]').exists()).toBe(false)
    })

    it('"x" düğmesi yalnızca kutuda metin varken görünür', async () => {
      serve()
      const w = await mountDirectory()
      expect(w.find('[data-testid="directory-search-clear"]').exists()).toBe(false)

      await typeSearch(w, 'a')

      expect(w.find('[data-testid="directory-search-clear"]').exists()).toBe(true)
    })

    it('arama kutusu 100 karakterle sınırlıdır (sunucuyla aynı)', async () => {
      serve()
      const w = await mountDirectory()

      expect(byId(w, 'directory-search').attributes('maxlength')).toBe('100')
    })

    it('arama, şehir ve uygunluk birlikte tek istekte gider', async () => {
      serve()
      const w = await mountDirectory()
      await byId(w, 'directory-city').setValue('Adana')
      await byId(w, 'directory-email').setValue('eligible')
      await typeSearch(w, 'ada')
      await byId(w, 'directory-filters').trigger('submit')
      await flushPromises()

      expect(Object.fromEntries(lastParams())).toEqual({
        page: '1',
        limit: '10',
        search: 'ada',
        city: 'Adana',
        email: 'eligible',
      })
    })

    it('zamanlayıcı bekleyen bileşen kaldırılınca istek yapılmaz', async () => {
      serve()
      const w = await mountDirectory()
      await typeSearch(w, 'ada')

      w.unmount()
      await vi.advanceTimersByTimeAsync(500)

      expect(directoryCalls()).toHaveLength(1)
    })
  })

  describe('sayfalama', () => {
    it('shadcn sayfalaması sayfa numaralarını gösterir ve 2. sayfayı ister', async () => {
      serve()
      const w = await mountDirectory()

      expect(w.findAll('[data-slot="pagination-item"]').map((b) => b.text())).toEqual([
        '1',
        '2',
        '3',
      ])
      await w.get('[aria-label="Sonraki sayfa"]').trigger('click')
      await flushPromises()

      expect(lastParams().get('page')).toBe('2')
      expect(rows(w)).toHaveLength(10)
      expect(w.get('[aria-current="page"]').text()).toBe('2')
    })

    it('süzgeçli toplam sayfalamayı belirler', async () => {
      serve((p) => (p.get('city') === 'Adana' ? 4 : 25))
      const w = await mountDirectory()
      expect(w.findAll('[data-slot="pagination-item"]')).toHaveLength(3)

      await byId(w, 'directory-city').setValue('Adana')
      await flushPromises()

      expect(byId(w, 'directory-total').text()).toBe('4 kişi')
      expect(rows(w)).toHaveLength(4)
      expect(w.find('[data-slot="pagination"]').exists()).toBe(false) // tek sayfa
    })

    it('yeni sayfa yüklenirken eski satırlar görünür kalır (aria-busy)', async () => {
      serve()
      const w = await mountDirectory()
      const pending = deferred<Response>()
      fetchMock.mockReturnValue(pending.promise)

      await w.get('[aria-label="Sonraki sayfa"]').trigger('click')
      await flushPromises()

      expect(rows(w)).toHaveLength(10)
      expect(byId(w, 'directory-page').attributes('aria-busy')).toBe('true')
      pending.resolve(
        jsonResponse(200, audiencePageBody({ page: 2, total: 25, items: members(10, 11) })),
      )
      await flushPromises()
      expect(byId(w, 'directory-page').attributes('aria-busy')).toBe('false')
    })

    it('veri kısalıp istenen sayfa boş kalırsa son sayfaya geçer', async () => {
      serve()
      const w = await mountDirectory()
      await w.get('[aria-label="Sayfa 2"]').trigger('click')
      await flushPromises()
      // Bu arada liste 12 kişiye indi: sayfa 3 artık yok.
      fetchMock.mockImplementation((input) => {
        const page = Number(new URL(String(input), 'http://x').searchParams.get('page'))
        const items = page === 1 ? members(10, 1) : page === 2 ? members(2, 11) : []
        return Promise.resolve(jsonResponse(200, audiencePageBody({ page, total: 12, items })))
      })

      await w.get('[aria-label="Sayfa 3"]').trigger('click')
      await flushPromises()

      expect(
        directoryCalls()
          .slice(-2)
          .map((u) => u.searchParams.get('page')),
      ).toEqual(['3', '2'])
      expect(byId(w, 'directory-total').text()).toBe('12 kişi')
      expect(rows(w)).toHaveLength(2)
      expect(w.get('[aria-current="page"]').text()).toBe('2')
      expect(w.find('[data-testid="directory-empty"]').exists()).toBe(false)
    })
  })

  describe('boş sonuç ve hata', () => {
    it('süzgeçsiz ve boşsa "Henüz kişi yok" der', async () => {
      fetchMock.mockImplementation((input) =>
        Promise.resolve(
          String(input).includes('/cities')
            ? jsonResponse(200, { cities: [] })
            : jsonResponse(200, audiencePageBody({ items: [], total: 0 })),
        ),
      )
      const w = await mountDirectory()

      expect(byId(w, 'directory-empty').text()).toContain('Henüz kişi yok')
      expect(w.find('table').exists()).toBe(false)
      expect(w.find('[data-slot="pagination"]').exists()).toBe(false)
    })

    it('süzgeç sonuç bırakmazsa nedenini söyler ve temizleme sunar; süzgeçler yerinde kalır', async () => {
      serve((p) => (p.get('city') ? 0 : 25))
      const w = await mountDirectory()

      await byId(w, 'directory-city').setValue('Adana')
      await flushPromises()

      const empty = byId(w, 'directory-empty')
      expect(empty.text()).toContain('Filtrelere uyan kişi yok')
      expect(empty.text()).toContain('Filtreleri temizle')
      expect(w.find('[data-testid="directory-filters"]').exists()).toBe(true)
      expect((byId(w, 'directory-city').element as HTMLSelectElement).value).toBe('Adana')

      await empty.get('button').trigger('click')
      await flushPromises()
      expect(rows(w)).toHaveLength(10)
    })

    it('hata panelinde başlık ve "Yeniden dene" çıkar; deneyince listeyi yükler', async () => {
      fetchMock.mockImplementation((input) =>
        Promise.resolve(
          String(input).includes('/cities')
            ? jsonResponse(200, { cities: CITIES })
            : errorResponse(500, 'INTERNAL'),
        ),
      )
      const w = await mountDirectory()

      const panel = w.get('[data-testid="error-panel"]')
      expect(panel.text()).toContain('Kişi listesi yüklenemedi.')
      expect(rows(w)).toHaveLength(0)
      expect(w.find('[data-testid="directory-filters"]').exists()).toBe(true) // süzgeçler kalır

      serve()
      await w.get('[data-testid="error-retry"]').trigger('click')
      await flushPromises()

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(false)
      expect(rows(w)).toHaveLength(10)
    })

    it('geçersiz süzgeç hatasında (400) "Yeniden dene" sunmaz', async () => {
      fetchMock.mockImplementation((input) =>
        Promise.resolve(
          String(input).includes('/cities')
            ? jsonResponse(200, { cities: CITIES })
            : errorResponse(400, 'INVALID_REQUEST'),
        ),
      )
      const w = await mountDirectory()

      expect(w.find('[data-testid="error-panel"]').exists()).toBe(true)
      expect(w.find('[data-testid="error-retry"]').exists()).toBe(false)
    })

    it('geç gelen eski yanıt yeni süzgecin sonucunu ezmez', async () => {
      serve()
      const w = await mountDirectory()
      const slow = deferred<Response>()
      fetchMock.mockImplementationOnce(() => slow.promise)

      await byId(w, 'directory-city').setValue('Adana') // yavaş
      await flushPromises()
      serve((p) => (p.get('city') === 'İzmir' ? 3 : 25))
      await byId(w, 'directory-city').setValue('İzmir') // hızlı
      await flushPromises()
      slow.resolve(jsonResponse(200, audiencePageBody({ total: 99, items: [memberBody()] })))
      await flushPromises()

      expect(byId(w, 'directory-total').text()).toBe('3 kişi')
    })

    it('bileşen kaldırılınca süren isteği iptal eder', async () => {
      fetchMock.mockReturnValue(deferred<Response>().promise)
      const w = mount(AudienceDirectory)
      await flushPromises()

      w.unmount()

      const signals = fetchMock.mock.calls.map(([, init]) => init?.signal)
      expect(signals.every((s) => s?.aborted)).toBe(true)
    })
  })

  describe('satırlar', () => {
    it('eksik bilgiyi "yok" yazar ve izinsiz kişiyi uygun değil gösterir', async () => {
      const person = memberBody({
        first_name: null,
        email: null,
        phone: null,
        sms_status: 'no_consent',
        email_status: 'no_address',
      })
      fetchMock.mockImplementation((input) =>
        Promise.resolve(
          String(input).includes('/cities')
            ? jsonResponse(200, { cities: [] })
            : jsonResponse(200, audiencePageBody({ items: [person] })),
        ),
      )
      const w = await mountDirectory()

      expect(w.findAll('[data-missing="true"]').map((m) => m.text())).toEqual([
        'Ad yok',
        'E-posta yok',
        'Telefon yok',
      ])
      expect(w.get('[data-testid="member-sms"] [data-status]').text()).toBe('İzin yok')
      expect(w.get('[data-testid="member-email-status"] [data-status]').text()).toBe('Adres yok')
    })
  })
})
