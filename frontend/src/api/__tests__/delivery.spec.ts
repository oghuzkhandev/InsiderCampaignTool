import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../campaigns'
import { isCampaignStatus, streamDeliverySnapshots, type DeliverySnapshot } from '../delivery'
import {
  controlledSse,
  errorResponse,
  sseEvent,
  sseResponse,
  type ControlledSse,
} from '@/__tests__/helpers'

const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'
const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => vi.unstubAllGlobals())

/** Backend'in yazdığı biçimde geçerli bir durum görüntüsü (gövde). */
function body(overrides: Record<string, unknown> = {}) {
  return {
    status: 'sending',
    messages: { queued: 3, retrying: 1, sending: 2, sent: 40, failed: 2, skipped: 1 },
    failures: { exhausted: 1, rejected: 1 },
    ...overrides,
  }
}

const snapshot = (data: unknown) => sseEvent('snapshot', data)

/** Akış bitene (hata) kadar gelen görüntüleri ve sonucu toplar. */
async function collect(signal?: AbortSignal) {
  const received: DeliverySnapshot[] = []
  let error: unknown
  try {
    await streamDeliverySnapshots(ID, (s) => received.push(s), signal)
  } catch (err) {
    error = err
  }
  return { received, error }
}

describe('istek', () => {
  it('olay akışını kampanya kimliğiyle ister', async () => {
    fetchMock.mockResolvedValue(sseResponse([]))
    const controller = new AbortController()
    await collect(controller.signal)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(url).toBe(`/api/campaigns/${ID}/events`)
    expect(init?.method ?? 'GET').toBe('GET')
    expect(new Headers(init?.headers).get('Accept')).toBe('text/event-stream')
    expect(init?.signal).toBe(controller.signal)
  })

  it('kimliği adrese güvenli yazar', async () => {
    fetchMock.mockResolvedValue(sseResponse([]))
    await streamDeliverySnapshots('a/b?c', () => undefined).catch(() => undefined)
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/campaigns/a%2Fb%3Fc/events')
  })
})

describe('durum görüntüleri', () => {
  it('her görüntüyü sırayla ve doğrulanmış tiplerle verir', async () => {
    fetchMock.mockResolvedValue(
      sseResponse([
        snapshot(body()),
        snapshot(
          body({
            status: 'completed',
            messages: { queued: 0, retrying: 0, sending: 0, sent: 44, failed: 2, skipped: 1 },
          }),
        ),
      ]),
    )
    const { received } = await collect()

    expect(received).toHaveLength(2)
    expect(received[0]).toEqual({
      status: 'sending',
      messages: { queued: 3, retrying: 1, sending: 2, sent: 40, failed: 2, skipped: 1 },
      failures: { exhausted: 1, rejected: 1 },
    })
    expect(received[1]?.status).toBe('completed')
    expect(received[1]?.messages.sent).toBe(44)
  })

  it('bilinmeyen olayları, yorum satırlarını ve fazladan alanları yok sayar', async () => {
    fetchMock.mockResolvedValue(
      sseResponse([
        ': keepalive\n\n',
        sseEvent('baska-olay', { x: 1 }),
        snapshot({
          ...body(),
          gelecekteki_alan: 'yok sayılır',
          messages: { ...body().messages, x: 9 },
        }),
      ]),
    )
    const { received } = await collect()
    expect(received).toHaveLength(1)
    expect(received[0]).toEqual(body())
  })

  it('parçalara bölünmüş akışı birleştirir', async () => {
    const text = snapshot(body())
    fetchMock.mockResolvedValue(
      sseResponse([text.slice(0, 17), text.slice(17, 40), text.slice(40)]),
    )
    const { received } = await collect()
    expect(received).toEqual([body()])
  })

  it('neden sayıları toplamı başarısız sayısına eşitse kabul eder', async () => {
    fetchMock.mockResolvedValue(
      sseResponse([
        snapshot(
          body({
            messages: { queued: 0, retrying: 0, sending: 0, sent: 5, failed: 2, skipped: 0 },
            failures: { exhausted: 1, rejected: 1 },
          }),
        ),
      ]),
    )
    expect((await collect()).received).toHaveLength(1)
  })

  const invalid: [string, unknown][] = [
    ['nesne değil (dizi)', []],
    ['nesne değil (null)', null],
    ['durum yok', { ...body(), status: undefined }],
    ['bilinmeyen durum', body({ status: 'paused' })],
    ['durum metin değil', body({ status: 3 })],
    ['messages yok', { status: 'sending', failures: { exhausted: 0, rejected: 0 } }],
    ['failures yok', { status: 'sending', messages: body().messages }],
    ['messages dizi', body({ messages: [] })],
    [
      'eksik sayı alanı',
      body({ messages: { queued: 1, retrying: 0, sending: 0, sent: 0, failed: 0 } }),
    ],
    ['negatif sayı', body({ messages: { ...body().messages, queued: -1 } })],
    ['ondalıklı sayı', body({ messages: { ...body().messages, sent: 1.5 } })],
    ['sayı yerine metin', body({ messages: { ...body().messages, sent: '40' } })],
    ['sayı yerine null', body({ messages: { ...body().messages, sent: null } })],
    ['NaN gibi geçersiz sayı', body({ messages: { ...body().messages, sent: 'NaN' } })],
    ['neden alanı eksik', body({ failures: { exhausted: 1 } })],
    ['negatif neden sayısı', body({ failures: { exhausted: -1, rejected: 0 } })],
    [
      'nedenlerin toplamı başarısızdan büyük',
      body({
        messages: { queued: 0, retrying: 0, sending: 0, sent: 0, failed: 1, skipped: 0 },
        failures: { exhausted: 1, rejected: 1 },
      }),
    ],
  ]
  it.each(invalid)('geçersiz görüntü reddedilir: %s', async (_name, data) => {
    fetchMock.mockResolvedValue(sseResponse([snapshot(data)]))
    const { received, error } = await collect()

    expect(received).toHaveLength(0) // geçersiz veri ekrana ulaşmaz
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'UNEXPECTED_RESPONSE', status: 200 })
  })

  it('JSON olmayan veriyi reddeder', async () => {
    fetchMock.mockResolvedValue(sseResponse(['event: snapshot\ndata: {bozuk\n\n']))
    const { received, error } = await collect()
    expect(received).toHaveLength(0)
    expect(error).toMatchObject({ code: 'UNEXPECTED_RESPONSE' })
  })

  it('geçerli görüntülerden sonra gelen geçersiz görüntüde durur; öncekiler iletilmiş olur', async () => {
    fetchMock.mockResolvedValue(sseResponse([snapshot(body()), snapshot({ status: 'x' })]))
    const { received, error } = await collect()
    expect(received).toHaveLength(1)
    expect(error).toMatchObject({ code: 'UNEXPECTED_RESPONSE' })
  })
})

describe('akışın sonu ve hatalar', () => {
  it('sunucu akışı kapatırsa STREAM_INTERRUPTED (görüntüler yine de iletilir)', async () => {
    fetchMock.mockResolvedValue(sseResponse([snapshot(body())]))
    const { received, error } = await collect()

    expect(received).toHaveLength(1)
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'STREAM_INTERRUPTED', status: 200 })
  })

  it('akış ortasında ağ koparsa STREAM_INTERRUPTED', async () => {
    let sse: ControlledSse | undefined
    fetchMock.mockImplementation(async (_url, init) => {
      sse = controlledSse(init?.signal)
      sse.push(snapshot(body()))
      return sse.response
    })
    const pending = collect()
    await vi.waitFor(() => expect(sse).toBeDefined())
    sse?.fail(new TypeError('network error'))

    const { received, error } = await pending
    expect(received).toHaveLength(1)
    expect(error).toMatchObject({ code: 'STREAM_INTERRUPTED' })
  })

  it('sunucunun error olayını ApiError yapar', async () => {
    fetchMock.mockResolvedValue(
      sseResponse([sseEvent('error', { code: 'INTERNAL', message: 'Sunucu mesajı' })]),
    )
    const { error } = await collect()
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'INTERNAL', message: 'Sunucu mesajı' })
  })

  it('biçimi bozuk error olayı UNEXPECTED_RESPONSE olur', async () => {
    fetchMock.mockResolvedValue(sseResponse([sseEvent('error', { mesaj: 'kod yok' })]))
    const { error } = await collect()
    expect(error).toMatchObject({ code: 'UNEXPECTED_RESPONSE' })
  })

  it('akış başlamadan önceki JSON hatayı ApiError yapar (kampanya yok)', async () => {
    fetchMock.mockResolvedValue(errorResponse(404, 'CAMPAIGN_NOT_FOUND'))
    const { received, error } = await collect()
    expect(received).toHaveLength(0)
    expect(error).toMatchObject({ code: 'CAMPAIGN_NOT_FOUND', status: 404 })
  })

  it('JSON olmayan hata yanıtı (ör. proxy 502 sayfası) UNEXPECTED_RESPONSE olur', async () => {
    fetchMock.mockResolvedValue(new Response('<html>Bad Gateway</html>', { status: 502 }))
    const { error } = await collect()
    expect(error).toMatchObject({ code: 'UNEXPECTED_RESPONSE', status: 502 })
  })

  it('gövdesiz başarılı yanıtı reddeder', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 200 }))
    const { error } = await collect()
    expect(error).toMatchObject({ code: 'UNEXPECTED_RESPONSE' })
  })

  it('ağ hatasını NETWORK yapar', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
    const { error } = await collect()
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'NETWORK', status: 0 })
  })
})

describe('iptal', () => {
  it('istek öncesi iptal AbortError olarak aynen iletilir (ağ hatası sayılmaz)', async () => {
    const abort = new DOMException('Aborted', 'AbortError')
    fetchMock.mockRejectedValue(abort)
    const { error } = await collect()
    expect(error).toBe(abort)
  })

  it('akış sürerken iptal edilirse AbortError verir (ağ hatası veya kopma sayılmaz)', async () => {
    let sse: ControlledSse | undefined
    fetchMock.mockImplementation(async (_url, init) => {
      sse = controlledSse(init?.signal)
      sse.push(snapshot(body()))
      return sse.response
    })
    const controller = new AbortController()
    const pending = collect(controller.signal)
    await vi.waitFor(() => expect(sse).toBeDefined())
    controller.abort()

    const { received, error } = await pending
    expect(received).toHaveLength(1)
    expect(error).toBeInstanceOf(DOMException)
    expect(error).toMatchObject({ name: 'AbortError' })
  })
})

describe('isCampaignStatus', () => {
  it('yalnızca bilinen durumları kabul eder', () => {
    for (const status of ['draft', 'approved', 'sending', 'completed']) {
      expect(isCampaignStatus(status)).toBe(true)
    }
    for (const value of ['failed', '', 'Draft', null, undefined, 1, {}]) {
      expect(isCampaignStatus(value)).toBe(false)
    }
  })
})
