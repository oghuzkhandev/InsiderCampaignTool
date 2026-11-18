import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../campaigns'
import { approveCampaign, generateContent, getCampaign, saveContent, type Delta } from '../content'
import {
  campaignBody,
  contentBody,
  controlledSse,
  errorResponse,
  jsonResponse,
  sseEvent,
  sseResponse,
} from '@/__tests__/helpers'

const fetchMock = vi.fn<typeof fetch>()
const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => vi.unstubAllGlobals())

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const outcome = await promise.then(
    () => undefined,
    (err: unknown) => err,
  )
  if (!(outcome instanceof ApiError)) throw new Error('ApiError bekleniyordu')
  return outcome
}

describe('getCampaign', () => {
  it('içeriği ve onay bilgisini okur', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, {
        ...campaignBody({ status: 'approved' }),
        content: contentBody({ content_version: 2, source: 'human' }),
        approved_content_version: 2,
        approved_at: '2026-11-18T11:00:00Z',
      }),
    )

    const detail = await getCampaign(ID)

    expect(fetchMock.mock.calls[0]![0]).toBe(`/api/campaigns/${ID}`)
    expect(detail.content?.content_version).toBe(2)
    expect(detail.content?.source).toBe('human')
    expect(detail.approved_content_version).toBe(2)
    expect(detail.approved_at).toBe('2026-11-18T11:00:00Z')
  })

  it('içerik yoksa alanlar boş kalır', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, campaignBody()))
    const detail = await getCampaign(ID)
    expect(detail.content).toBeUndefined()
    expect(detail.approved_content_version).toBeUndefined()
  })

  it('geçersiz içerik veya onay alanında beklenmeyen yanıt hatası verir', async () => {
    for (const extra of [
      { content: { content_version: 0, source: 'llm' } },
      { content: contentBody({ source: 'bot' as never }) },
      { content: 'x' },
      { approved_content_version: 1 }, // approved_at eksik
      { approved_content_version: -1, approved_at: 'x' },
    ]) {
      fetchMock.mockResolvedValue(jsonResponse(200, { ...campaignBody(), ...extra }))
      expect((await rejection(getCampaign(ID))).code).toBe('UNEXPECTED_RESPONSE')
    }
  })

  it('404 ve ağ hatasını ApiError yapar; kimliği URL için kodlar', async () => {
    fetchMock.mockResolvedValue(errorResponse(404, 'CAMPAIGN_NOT_FOUND'))
    expect((await rejection(getCampaign('a b/c'))).code).toBe('CAMPAIGN_NOT_FOUND')
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/campaigns/a%20b%2Fc')

    fetchMock.mockRejectedValue(new TypeError('failed'))
    expect((await rejection(getCampaign(ID))).code).toBe('NETWORK')
  })
})

describe('saveContent', () => {
  it('yalnızca üç alanı PUT eder ve yeni sürümü döndürür', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, contentBody({ content_version: 3, source: 'human' })),
    )

    const version = await saveContent(ID, {
      sms_text: 'a',
      email_subject: 'b',
      email_body: 'c',
      ...({ extra: 'x' } as object),
    })

    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe(`/api/campaigns/${ID}/content`)
    expect(init?.method).toBe('PUT')
    expect(JSON.parse(String(init?.body))).toEqual({
      sms_text: 'a',
      email_subject: 'b',
      email_body: 'c',
    })
    expect(version.content_version).toBe(3)
  })

  it('doğrulama sebeplerini ve 409 durumunu ApiError olarak verir', async () => {
    fetchMock.mockResolvedValue(
      errorResponse(422, 'CONTENT_INVALID', [{ code: 'TOO_LONG', field: 'sms_text' }]),
    )
    const err = await rejection(
      saveContent(ID, { sms_text: 'a', email_subject: 'b', email_body: 'c' }),
    )
    expect(err.status).toBe(422)
    expect(err.reasons).toEqual([
      { code: 'TOO_LONG', field: 'sms_text', text: undefined, count: undefined },
    ])

    fetchMock.mockResolvedValue(errorResponse(409, 'CAMPAIGN_NOT_DRAFT'))
    expect(
      (await rejection(saveContent(ID, { sms_text: 'a', email_subject: 'b', email_body: 'c' })))
        .code,
    ).toBe('CAMPAIGN_NOT_DRAFT')
  })

  it('geçersiz başarılı yanıtı reddeder', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { content_version: 1 }))
    expect(
      (await rejection(saveContent(ID, { sms_text: 'a', email_subject: 'b', email_body: 'c' })))
        .code,
    ).toBe('UNEXPECTED_RESPONSE')
  })
})

describe('generateContent', () => {
  const deltaEvents = [
    sseEvent('delta', { field: 'sms_text', text: 'Mer', replace: true }),
    sseEvent('delta', { field: 'sms_text', text: 'haba' }),
    sseEvent('delta', { field: 'email_subject', text: 'Konu', replace: true }),
  ]

  it('parçaları sırayla verir ve validated ile sürümü döndürür', async () => {
    fetchMock.mockResolvedValue(sseResponse([...deltaEvents, sseEvent('validated', contentBody())]))
    const deltas: Delta[] = []

    const version = await generateContent(ID, (d) => deltas.push(d))

    expect(deltas).toEqual([
      { field: 'sms_text', text: 'Mer', replace: true },
      { field: 'sms_text', text: 'haba', replace: false },
      { field: 'email_subject', text: 'Konu', replace: true },
    ])
    expect(version.content_version).toBe(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe(`/api/campaigns/${ID}/content`)
    expect(init?.method).toBe('POST')
    expect(init?.body).toBeUndefined()
  })

  it('olaylar parça sınırlarında bölünse de çalışır', async () => {
    const all = [...deltaEvents, sseEvent('validated', contentBody())].join('')
    fetchMock.mockImplementation(async () => sseResponse(all.split('')))
    const deltas: Delta[] = []
    expect((await generateContent(ID, (d) => deltas.push(d))).content_version).toBe(1)
    expect(deltas).toHaveLength(3)
  })

  it('error olayını sebepler ve doğrulanmamış metinlerle ApiError yapar', async () => {
    fetchMock.mockResolvedValue(
      sseResponse([
        deltaEvents[0]!,
        sseEvent('error', {
          code: 'CONTENT_INVALID',
          message: 'Sunucu mesajı',
          reasons: [{ code: 'TOO_LONG', field: 'sms_text' }],
          partial: { sms_text: 'çok uzun', email_subject: 'Konu', email_body: '' },
        }),
      ]),
    )

    const err = await rejection(generateContent(ID, () => {}))

    expect(err.code).toBe('CONTENT_INVALID')
    expect(err.reasons).toEqual([
      { code: 'TOO_LONG', field: 'sms_text', text: undefined, count: undefined },
    ])
    expect(err.partial).toEqual({ sms_text: 'çok uzun', email_subject: 'Konu', email_body: '' })
  })

  it('partial yoksa veya eksikse partial verilmez', async () => {
    for (const partial of [undefined, { sms_text: 'a' }, 'x']) {
      fetchMock.mockResolvedValue(
        sseResponse([sseEvent('error', { code: 'LLM_TIMEOUT', message: 'm', partial })]),
      )
      const err = await rejection(generateContent(ID, () => {}))
      expect(err.code).toBe('LLM_TIMEOUT')
      expect(err.partial).toBeUndefined()
      expect(err.retryable).toBe(true)
    }
  })

  it('validated ya da error gelmeden biten akışı STREAM_INTERRUPTED sayar', async () => {
    fetchMock.mockResolvedValue(sseResponse([deltaEvents[0]!, 'event: delta\ndata: {"fie']))
    const err = await rejection(generateContent(ID, () => {}))
    expect(err.code).toBe('STREAM_INTERRUPTED')
    expect(err.retryable).toBe(true)
  })

  it('akış ortasında bağlantı koparsa STREAM_INTERRUPTED verir', async () => {
    const sse = controlledSse()
    fetchMock.mockResolvedValue(sse.response)
    const promise = generateContent(ID, () => {})
    sse.push(deltaEvents[0]!)
    sse.fail(new TypeError('network error'))
    expect((await rejection(promise)).code).toBe('STREAM_INTERRUPTED')
  })

  it('iptalde AbortError olduğu gibi fırlatılır', async () => {
    const controller = new AbortController()
    fetchMock.mockImplementation(async (_url, init) => controlledSse(init?.signal).response)
    const promise = generateContent(ID, () => {}, controller.signal)
    controller.abort()
    await expect(promise).rejects.toMatchObject({ name: 'AbortError' })

    fetchMock.mockRejectedValue(new DOMException('Aborted', 'AbortError'))
    await expect(generateContent(ID, () => {}, controller.signal)).rejects.toMatchObject({
      name: 'AbortError',
    })
  })

  it('akış başlamadan önceki JSON hataları (409, 404) ApiError olur', async () => {
    fetchMock.mockResolvedValue(errorResponse(409, 'CAMPAIGN_NOT_DRAFT'))
    expect((await rejection(generateContent(ID, () => {}))).code).toBe('CAMPAIGN_NOT_DRAFT')
    fetchMock.mockResolvedValue(errorResponse(404, 'CAMPAIGN_NOT_FOUND'))
    expect((await rejection(generateContent(ID, () => {}))).code).toBe('CAMPAIGN_NOT_FOUND')
    fetchMock.mockResolvedValue(new Response('<html>Bad Gateway</html>', { status: 502 }))
    expect((await rejection(generateContent(ID, () => {}))).code).toBe('UNEXPECTED_RESPONSE')
    fetchMock.mockRejectedValue(new TypeError('failed'))
    expect((await rejection(generateContent(ID, () => {}))).code).toBe('NETWORK')
  })

  it('geçersiz delta veya validated verisinde beklenmeyen yanıt hatası verir', async () => {
    for (const bad of [
      sseEvent('delta', { field: 'bilinmeyen', text: 'x' }),
      sseEvent('delta', { field: 'sms_text', text: 5 }),
      sseEvent('validated', { content_version: 1 }),
      'event: delta\ndata: json-degil\n\n',
    ]) {
      fetchMock.mockResolvedValue(sseResponse([bad]))
      expect((await rejection(generateContent(ID, () => {}))).code).toBe('UNEXPECTED_RESPONSE')
    }
  })

  it('bilinmeyen olayları yok sayar ve validated sonrasını okumaz', async () => {
    const sse = controlledSse()
    fetchMock.mockResolvedValue(sse.response)
    const promise = generateContent(ID, () => {})
    sse.push(
      sseEvent('ping', {}) +
        sseEvent('validated', contentBody()) +
        sseEvent('error', { code: 'INTERNAL' }),
    )

    expect((await promise).content_version).toBe(1)
    expect(sse.cancelled).toBe(true) // gerisi okunmaz, akış kapatılır
  })
})

describe('approveCampaign', () => {
  const approved = {
    id: ID,
    status: 'approved',
    approved_content_version: 2,
    approved_at: '2026-11-18T11:00:00Z',
    queued: { sms: 133, email: 150, total: 283 },
  }

  it('content_version gönderir ve kuyruğa alınan sayıları döndürür', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, approved))

    const result = await approveCampaign(ID, 2)

    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe(`/api/campaigns/${ID}/approve`)
    expect(init?.method).toBe('POST')
    expect(JSON.parse(String(init?.body))).toEqual({ content_version: 2 })
    expect(result.queued).toEqual({ sms: 133, email: 150, total: 283 })
    expect(result.approved_content_version).toBe(2)
  })

  it('409 ve 422 sebep kodlarını (sayı ve örnek ad dahil) taşır', async () => {
    for (const code of [
      'STALE_CONTENT_VERSION',
      'NO_VALIDATED_CONTENT',
      'CAMPAIGN_ALREADY_APPROVED',
    ]) {
      fetchMock.mockResolvedValue(errorResponse(409, code))
      const err = await rejection(approveCampaign(ID, 1))
      expect([err.status, err.code]).toEqual([409, code])
    }
    fetchMock.mockResolvedValue(
      jsonResponse(422, {
        error: {
          code: 'RENDER_LIMIT_EXCEEDED',
          message: 'm',
          reasons: [{ code: 'TOO_LONG', field: 'sms_text', count: 12, text: 'Zeynep' }],
        },
      }),
    )
    const err = await rejection(approveCampaign(ID, 1))
    expect(err.reasons).toEqual([
      { code: 'TOO_LONG', field: 'sms_text', count: 12, text: 'Zeynep' },
    ])
  })

  it('geçersiz başarılı yanıtı reddeder', async () => {
    for (const bad of [
      {},
      { ...approved, queued: { sms: 1 } },
      { ...approved, approved_content_version: 0.5 },
    ]) {
      fetchMock.mockResolvedValue(jsonResponse(200, bad))
      expect((await rejection(approveCampaign(ID, 1))).code).toBe('UNEXPECTED_RESPONSE')
    }
  })
})
