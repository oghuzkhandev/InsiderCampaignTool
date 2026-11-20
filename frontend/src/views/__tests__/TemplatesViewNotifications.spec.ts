import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import TemplatesView from '../TemplatesView.vue'
import { errorResponse, jsonResponse } from '@/__tests__/helpers'
import { notify } from '@/notify/notify'

// Şablon eylemlerinin (kaydet, sil) sonucu bir bildirimdir: başarı, hata. Gerçek API işlevleri
// `fetch` taklidi üzerinden çalışır.
const fetchMock = vi.fn<typeof fetch>()
const template = {
  id: 'template-1',
  name: 'Sepet',
  sms_text: 'Merhaba {{first_name}}',
  email_subject: 'Konu',
  email_body: 'Gövde',
  created_at: '2026-11-20T09:00:00Z',
  updated_at: '2026-11-20T09:00:00Z',
}
const page = (items: unknown[]) =>
  jsonResponse(200, { items, total: items.length, page: 1, limit: 10 })

let handlers: Record<string, () => Response> = {}
let wrapper: VueWrapper | null = null

beforeEach(() => {
  handlers = { 'GET /api/templates?page=1&limit=10': () => page([template]) }
  fetchMock.mockReset()
  fetchMock.mockImplementation(async (input, init) => {
    const key = `${init?.method ?? 'GET'} ${String(input)}`
    const handler = handlers[key]
    if (!handler) throw new Error(`beklenmeyen istek: ${key}`)
    return handler()
  })
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

const spies = () => ({
  success: vi.spyOn(notify, 'success').mockReturnValue('s'),
  error: vi.spyOn(notify, 'error').mockReturnValue('e'),
})

async function mountView() {
  wrapper = mount(TemplatesView, { attachTo: document.body })
  await flushPromises()
  return wrapper
}

const button = (w: VueWrapper, text: string) =>
  w.findAll('button').find((b) => b.text().includes(text))

async function editFirst(w: VueWrapper) {
  await button(w, 'Düzenle')!.trigger('click')
  await flushPromises()
}

describe('şablon eylem bildirimleri', () => {
  it('silinince başarı bildirimi verir', async () => {
    const n = spies()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    handlers['DELETE /api/templates/template-1'] = () => new Response(null, { status: 204 })
    const w = await mountView()
    handlers['GET /api/templates?page=1&limit=10'] = () => page([]) // silince liste yeniden okunur

    await button(w, 'Sil')!.trigger('click')
    await flushPromises()

    expect(n.success).toHaveBeenCalledTimes(1)
    expect(n.success.mock.calls[0]![0]).toBe('Şablon silindi')
    expect(n.success.mock.calls[0]![1]).toContain('Sepet')
    expect(n.error).not.toHaveBeenCalled()
  })

  it('silme onaylanmazsa istek yapılmaz ve bildirim çıkmaz', async () => {
    const n = spies()
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    const w = await mountView()
    const before = fetchMock.mock.calls.length

    await button(w, 'Sil')!.trigger('click')
    await flushPromises()

    expect(fetchMock.mock.calls.length).toBe(before)
    expect(n.success).not.toHaveBeenCalled()
    expect(n.error).not.toHaveBeenCalled()
  })

  it('silinemezse hata bildirimi verir', async () => {
    const n = spies()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    handlers['DELETE /api/templates/template-1'] = () => errorResponse(500, 'INTERNAL')
    const w = await mountView()

    await button(w, 'Sil')!.trigger('click')
    await flushPromises()

    expect(n.error).toHaveBeenCalledTimes(1)
    expect(n.error.mock.calls[0]![0]).toBe('Şablon silinemedi')
    expect(n.success).not.toHaveBeenCalled()
  })

  it('kaydedilince başarı bildirimi verir; sayfada ayrıca satır içi başarı metni kalmaz', async () => {
    const n = spies()
    handlers['PUT /api/templates/template-1'] = () =>
      jsonResponse(200, { ...template, name: 'Sepet 2' })
    const w = await mountView()
    await editFirst(w)

    await w.get('#template-name').setValue('Sepet 2')
    await w.get('form').trigger('submit')
    await flushPromises()

    expect(n.success).toHaveBeenCalledWith(
      'Şablon kaydedildi',
      'Kampanyanın içerik ekranında kullanabilirsiniz.',
    )
    expect(w.text()).not.toContain('Şablon kaydedildi')
  })

  it('kaydedilemezse hata bildirimi verir ve alan sorunları sayfada kalır', async () => {
    const n = spies()
    handlers['PUT /api/templates/template-1'] = () => errorResponse(422, 'INVALID_TEMPLATE_NAME')
    const w = await mountView()
    await editFirst(w)

    await w.get('#template-name').setValue('Başka ad')
    await w.get('form').trigger('submit')
    await flushPromises()

    expect(n.error).toHaveBeenCalledTimes(1)
    expect(n.error.mock.calls[0]![0]).toBe('Şablon kaydedilemedi')
    expect(n.success).not.toHaveBeenCalled()
    expect(w.find('[data-testid="error-panel"]').exists()).toBe(true)
  })

  it('değişiklik yokken kaydet bildirim üretmez', async () => {
    const n = spies()
    const w = await mountView()
    await editFirst(w)

    await w.get('form').trigger('submit')
    await flushPromises()

    expect(n.success).not.toHaveBeenCalled()
    expect(n.error).not.toHaveBeenCalled()
  })
})
