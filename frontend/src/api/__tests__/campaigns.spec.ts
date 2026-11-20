import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, createCampaign, listCampaigns, parseCampaignList } from '../campaigns'
import { campaignBody, errorResponse, jsonResponse } from '@/__tests__/helpers'

const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const err = await promise.then(
    () => undefined,
    (e: unknown) => e,
  )
  if (!(err instanceof ApiError)) throw new Error('istek ApiError ile reddedilmeliydi')
  return err
}

describe('createCampaign', () => {
  it('brief gövdesini JSON olarak /api/campaigns adresine POST eder', async () => {
    fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))
    const controller = new AbortController()

    await createCampaign('Bursa sepet', controller.signal)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/campaigns')
    expect(init?.method).toBe('POST')
    expect(init?.body).toBe(JSON.stringify({ brief: 'Bursa sepet' }))
    expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json')
    expect(init?.signal).toBe(controller.signal)
  })

  it('başarılı yanıtı kampanya olarak döndürür', async () => {
    fetchMock.mockResolvedValue(jsonResponse(201, campaignBody()))

    const c = await createCampaign('x')

    expect(c.id).toBe('498882b6-2221-4589-a99f-67ceb68c59c2')
    expect(c.status).toBe('draft')
    expect(c.audience).toEqual({ total: 190, sms: 140, email: 150 })
    expect(c.segment.all).toHaveLength(3)
  })

  it('sıfır kitle sayılarını geçerli kabul eder', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(201, campaignBody({ audience: { total: 0, sms: 0, email: 0 } })),
    )
    expect((await createCampaign('x')).audience.total).toBe(0)
  })

  it('SEGMENT_INVALID yanıtını sebepleriyle ApiError yapar', async () => {
    fetchMock.mockResolvedValue(
      errorResponse(422, 'SEGMENT_INVALID', [
        { code: 'UNSUPPORTED_FIELD', field: 'favorite_color' },
        { code: 'UNSUPPORTED_PART', text: 'ürün kategorisi' },
      ]),
    )

    const err = await rejection(createCampaign('x'))

    expect(err.status).toBe(422)
    expect(err.code).toBe('SEGMENT_INVALID')
    expect(err.reasons).toEqual([
      { code: 'UNSUPPORTED_FIELD', field: 'favorite_color', text: undefined },
      { code: 'UNSUPPORTED_PART', field: undefined, text: 'ürün kategorisi' },
    ])
    expect(err.retryable).toBe(false)
  })

  it('geçersiz biçimli sebepleri yok sayar', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(422, {
        error: {
          code: 'SEGMENT_INVALID',
          message: 'm',
          reasons: [null, 5, { field: 'x' }, { code: 'A' }],
        },
      }),
    )
    const err = await rejection(createCampaign('x'))
    expect(err.reasons).toEqual([{ code: 'A', field: undefined, text: undefined }])
  })

  it.each([
    ['LLM_TIMEOUT', 504, true],
    ['LLM_RATE_LIMITED', 503, true],
    ['INTERNAL', 500, true],
    ['SEGMENT_INVALID', 422, false],
    ['BRIEF_EMPTY', 422, false],
    ['BRIEF_TOO_LONG', 422, false],
    ['REQUEST_TOO_LARGE', 413, false],
  ])('%s yeniden denenebilirlik: %s', async (code, status, retryable) => {
    fetchMock.mockResolvedValue(errorResponse(status, code))
    const err = await rejection(createCampaign('x'))
    expect(err.code).toBe(code)
    expect(err.retryable).toBe(retryable)
  })

  it('JSON olmayan hata yanıtını (ör. proxy 502 HTML) beklenmeyen yanıt sayar', async () => {
    fetchMock.mockResolvedValue(new Response('<html>Bad Gateway</html>', { status: 502 }))

    const err = await rejection(createCampaign('x'))

    expect(err.status).toBe(502)
    expect(err.code).toBe('UNEXPECTED_RESPONSE')
    expect(err.retryable).toBe(true)
  })

  it('kodu olmayan hata gövdesini beklenmeyen yanıt sayar', async () => {
    fetchMock.mockResolvedValue(jsonResponse(500, { error: { message: 'x' } }))
    expect((await rejection(createCampaign('x'))).code).toBe('UNEXPECTED_RESPONSE')
  })

  it('ağ hatasını NETWORK olarak bildirir', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))

    const err = await rejection(createCampaign('x'))

    expect(err.code).toBe('NETWORK')
    expect(err.status).toBe(0)
    expect(err.retryable).toBe(true)
  })

  it('iptal hatasını olduğu gibi fırlatır', async () => {
    const abort = new DOMException('Aborted', 'AbortError')
    fetchMock.mockRejectedValue(abort)
    await expect(createCampaign('x')).rejects.toBe(abort)
  })

  it.each([
    ['gövde nesne değil', []],
    ['audience yok', { ...campaignBody(), audience: undefined }],
    ['negatif sayı', { ...campaignBody(), audience: { total: -1, sms: 0, email: 0 } }],
    ['ondalık sayı', { ...campaignBody(), audience: { total: 1.5, sms: 0, email: 0 } }],
    ['metin sayı', { ...campaignBody(), audience: { total: '5', sms: 0, email: 0 } }],
    ['segment.all yok', { ...campaignBody(), segment: {} }],
    [
      'koşul değeri nesne',
      { ...campaignBody(), segment: { all: [{ field: 'city', op: 'eq', value: {} }] } },
    ],
    [
      'koşul listesi sayı içeriyor',
      { ...campaignBody(), segment: { all: [{ field: 'city', op: 'in', value: ['a', 1] }] } },
    ],
    ['kimlik yok', { ...campaignBody(), id: undefined }],
  ])('başarılı ama bozuk yanıtı reddeder: %s', async (_name, body) => {
    fetchMock.mockResolvedValue(jsonResponse(201, body))
    const err = await rejection(createCampaign('x'))
    expect(err.code).toBe('UNEXPECTED_RESPONSE')
  })
})

describe('listCampaigns', () => {
  const sample = {
    id: '498882b6-2221-4589-a99f-67ceb68c59c2',
    brief: 'Örnek brief',
    status: 'draft',
    created_at: '2026-11-19T21:00:00Z',
  }

  it('GET /api/campaigns?limit=… adresini ister; varsayılan 10', async () => {
    fetchMock.mockImplementation(async () => jsonResponse(200, { campaigns: [] })) // her çağrıya yeni yanıt
    const controller = new AbortController()

    await listCampaigns(undefined, controller.signal)
    await listCampaigns(20)

    expect(fetchMock.mock.calls[0]![0]).toBe('/api/campaigns?limit=10')
    expect(fetchMock.mock.calls[0]![1]?.method).toBeUndefined() // GET
    expect(fetchMock.mock.calls[0]![1]?.signal).toBe(controller.signal)
    expect(fetchMock.mock.calls[1]![0]).toBe('/api/campaigns?limit=20')
  })

  it('satırları sırasıyla ve yalnızca belgelenen alanlarla döndürür', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, {
        campaigns: [sample, { ...sample, id: 'b', brief: 'İkinci', status: 'completed' }],
      }),
    )

    const rows = await listCampaigns()

    expect(rows).toEqual([sample, { ...sample, id: 'b', brief: 'İkinci', status: 'completed' }])
  })

  it('boş listeyi boş dizi olarak döndürür', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { campaigns: [] }))
    expect(await listCampaigns()).toEqual([])
  })

  it('sunucunun gövdesindeki fazladan alanları (kitle sayısı gibi) atar', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, { campaigns: [{ ...sample, audience: { total: 1, sms: 1, email: 1 } }] }),
    )
    const [row] = await listCampaigns()
    expect(row).toEqual(sample)
    expect(row).not.toHaveProperty('audience')
  })

  it('ağ hatasını NETWORK olarak bildirir', async () => {
    fetchMock.mockRejectedValue(new TypeError('failed'))
    const err = await rejection(listCampaigns())
    expect(err.code).toBe('NETWORK')
    expect(err.retryable).toBe(true)
  })

  it('iptali olduğu gibi fırlatır', async () => {
    fetchMock.mockRejectedValue(new DOMException('abort', 'AbortError'))
    await expect(listCampaigns()).rejects.toMatchObject({ name: 'AbortError' })
  })

  it('sunucu hatasını (geçersiz limit gibi) ApiError olarak verir', async () => {
    fetchMock.mockResolvedValue(errorResponse(400, 'INVALID_REQUEST'))
    const err = await rejection(listCampaigns(0))
    expect(err.status).toBe(400)
    expect(err.code).toBe('INVALID_REQUEST')
  })

  it('JSON olmayan 502 yanıtını beklenmeyen yanıt sayar', async () => {
    fetchMock.mockResolvedValue(new Response('<html>Bad gateway</html>', { status: 502 }))
    const err = await rejection(listCampaigns())
    expect(err.code).toBe('UNEXPECTED_RESPONSE')
  })

  it.each([
    ['gövde dizi', []],
    ['campaigns dizi değil', { campaigns: {} }],
    ['campaigns yok', {}],
    ['kimlik sayı', { campaigns: [{ ...sample, id: 1 }] }],
    ['brief yok', { campaigns: [{ ...sample, brief: undefined }] }],
    ['durum yok', { campaigns: [{ ...sample, status: null }] }],
    ['tarih yok', { campaigns: [{ ...sample, created_at: undefined }] }],
    ['tarih geçersiz', { campaigns: [{ ...sample, created_at: 'yarın' }] }],
    ['satır nesne değil', { campaigns: ['x'] }],
  ])('başarılı ama bozuk yanıtı reddeder: %s', async (_name, body) => {
    fetchMock.mockResolvedValue(jsonResponse(200, body))
    const err = await rejection(listCampaigns())
    expect(err.code).toBe('UNEXPECTED_RESPONSE')
  })

  it('parseCampaignList tek bozuk satırda tüm listeyi reddeder (kısmi liste göstermez)', () => {
    expect(() => parseCampaignList(200, { campaigns: [sample, { ...sample, id: 5 }] })).toThrow(
      ApiError,
    )
  })
})
