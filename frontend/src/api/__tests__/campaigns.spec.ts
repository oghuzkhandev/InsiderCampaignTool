import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, createCampaign } from '../campaigns'
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
