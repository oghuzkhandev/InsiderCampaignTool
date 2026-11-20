import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, type PropType } from 'vue'
import { RouterLinkStub, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import DeliveryDashboardView from '../DeliveryDashboardView.vue'
import {
  campaignBody,
  contentBody,
  controlledSse,
  jsonResponse,
  sseEvent,
  type ControlledSse,
} from '@/__tests__/helpers'
import { recipientBody, recipientPageBody } from '@/__tests__/recipient-helpers'

// Gönderim takibi: sağdaki kampanya bilgileri (gönderilen içerik, alıcılar kartı) ve "Alıcılar"
// çekmecesinin ekrana bağlanması. Çekmecenin kendi davranışı `RecipientsSheet.spec.ts`'te sınanır.
const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'
const fetchMock = vi.fn<typeof fetch>()
let sse: ControlledSse | null = null
let wrapper: VueWrapper | null = null
const calls: string[] = []

const FilterSelectStub = defineComponent({
  name: 'FilterSelect',
  inheritAttrs: false,
  props: {
    id: { type: String, required: true },
    modelValue: { type: String, required: true },
    options: { type: Array as PropType<{ value: string; label: string }[]>, required: true },
    allLabel: { type: String, required: true },
  },
  setup(props) {
    return () => h('select', { id: props.id })
  },
})

function serve(detail: Record<string, unknown>) {
  fetchMock.mockImplementation(async (input, init) => {
    const path = String(input)
    calls.push(path)
    if (path.endsWith('/events')) {
      sse = controlledSse(init?.signal)
      return sse.response
    }
    if (path.includes('/recipients')) {
      return jsonResponse(
        200,
        recipientPageBody({
          total: 100,
          items: [recipientBody({ first_name: 'Zeynep', address: 'zeynep@example.test' })],
        }),
      )
    }
    return jsonResponse(200, detail)
  })
}

const approvedDetail = (extra: Record<string, unknown> = {}) => ({
  ...campaignBody({ id: ID, status: 'approved' }),
  approved_content_version: 1,
  approved_at: '2026-11-19T10:00:00Z',
  ...extra,
})

async function mountView() {
  wrapper = mount(DeliveryDashboardView, {
    attachTo: document.body,
    props: { id: ID },
    global: { stubs: { RouterLink: RouterLinkStub, FilterSelect: FilterSelectStub } },
  })
  await flushPromises()
  return wrapper
}

const snapshot = (channels: boolean) =>
  sseEvent('snapshot', {
    status: 'sending',
    messages: { queued: 0, retrying: 0, sending: 0, sent: 100, failed: 0, skipped: 0 },
    failures: { exhausted: 0, rejected: 0 },
    ...(channels
      ? {
          channels: {
            sms: { queued: 0, retrying: 0, sending: 0, sent: 51, failed: 0, skipped: 0 },
            email: { queued: 0, retrying: 0, sending: 0, sent: 49, failed: 0, skipped: 0 },
          },
        }
      : {}),
  })

beforeEach(() => {
  fetchMock.mockReset()
  calls.length = 0
  sse = null
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

describe('gönderilen içerik', () => {
  it('SMS metni, e-posta konusu ve e-posta gövdesi sayfada görünür', async () => {
    serve(approvedDetail({ content: contentBody() }))
    const w = await mountView()

    const card = w.get('[data-testid="sent-content"]')
    expect(card.get('h2').text()).toBe('Gönderilen içerik')
    expect(card.get('[data-testid="preview-sms"]').text()).toContain('%30 indirim seni bekliyor')
    expect(card.get('[data-testid="preview-subject"]').text()).toBe('Sepetin seni bekliyor')
    // Kullanıcının asıl istediği: e-posta gövdesi.
    expect(card.get('[data-testid="preview-body"]').text()).toContain('Ürünlerin hâlâ burada.')
  })

  it('içerik yoksa kart çıkmaz', async () => {
    serve(approvedDetail())
    const w = await mountView()

    expect(w.find('[data-testid="sent-content"]').exists()).toBe(false)
  })

  it('onaylanmamış kampanyada kart "İçerik" adını taşır (henüz gönderilmedi)', async () => {
    serve({ ...campaignBody({ id: ID, status: 'draft' }), content: contentBody() })
    const w = await mountView()

    expect(w.get('[data-testid="sent-content"] h2').text()).toBe('İçerik')
  })
})

describe('alıcılar kartı', () => {
  it('onaylı kampanyada kart ve "Alıcıları görüntüle" düğmesi görünür; çekmece kapalıyken alıcı istenmez', async () => {
    serve(approvedDetail())
    const w = await mountView()

    expect(w.get('[data-testid="recipients-open"]').text()).toContain('Alıcıları görüntüle')
    expect(calls.some((c) => c.includes('/recipients'))).toBe(false)
  })

  it('canlı görüntü gelince toplam mesaj ve kanal dağılımı kartta görünür', async () => {
    serve(approvedDetail())
    const w = await mountView()

    sse!.push(snapshot(true))
    await flushPromises()

    const card = w.get('[data-testid="recipients-card"]')
    expect(card.get('[data-testid="recipients-count"]').text()).toContain('100')
    expect(card.text()).toContain('SMS 51')
    expect(card.text()).toContain('E-posta 49')
  })

  it('kanal kırılımı yoksa (eski sunucu) yalnızca toplam gösterilir', async () => {
    serve(approvedDetail())
    const w = await mountView()

    sse!.push(snapshot(false))
    await flushPromises()

    const card = w.get('[data-testid="recipients-card"]')
    expect(card.get('[data-testid="recipients-count"]').text()).toContain('100')
    expect(card.text()).not.toContain('SMS 51')
  })

  it('düğme çekmeceyi açar ve bu kampanyanın alıcılarını okur', async () => {
    serve(approvedDetail())
    const w = await mountView()

    await w.get('[data-testid="recipients-open"]').trigger('click')
    await flushPromises()

    expect(calls).toContain(`/api/campaigns/${ID}/recipients?page=1&limit=10`)
    const sheet = document.body.querySelector('[data-testid="recipients-sheet"]')
    expect(sheet).not.toBeNull()
    expect(sheet!.textContent).toContain('Zeynep')
    expect(sheet!.textContent).toContain('zeynep@example.test')
  })

  it('onaylanmamış kampanyada alıcılar kartı yoktur', async () => {
    serve({ ...campaignBody({ id: ID, status: 'draft' }) })
    const w = await mountView()

    expect(w.find('[data-testid="recipients-card"]').exists()).toBe(false)
  })
})

describe('yerleşim', () => {
  it('canlı durum solda, kampanya bilgileri sağdaki yan sütundadır', async () => {
    serve(approvedDetail({ content: contentBody() }))
    const w = await mountView()
    sse!.push(snapshot(true))
    await flushPromises()

    const main = w.get('[data-testid="delivery-main"]')
    const aside = w.get('[data-testid="delivery-aside"]')
    expect(main.find('[data-testid="delivery-summary"]').exists()).toBe(true)
    expect(aside.find('#campaign-title').exists()).toBe(true)
    expect(aside.find('[data-testid="recipients-card"]').exists()).toBe(true)
    expect(aside.find('[data-testid="sent-content"]').exists()).toBe(true)
    expect(main.find('[data-testid="recipients-card"]').exists()).toBe(false)
  })
})
