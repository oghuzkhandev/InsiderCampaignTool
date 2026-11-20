import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listScheduled, parseScheduledPage } from '../scheduled'
import { ApiError } from '../campaigns'
import { errorResponse, jsonResponse } from '@/__tests__/helpers'
import { scheduledRow } from '@/__tests__/scheduled-helpers'

const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  vi.unstubAllGlobals()
})

function pageBody(over: Record<string, unknown> = {}) {
  return {
    items: [scheduledRow()],
    total: 1,
    page: 1,
    limit: 10,
    server_time: '2026-11-21T09:00:00Z',
    ...over,
  }
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const err = await promise.then(
    () => undefined,
    (e: unknown) => e,
  )
  if (!(err instanceof ApiError)) throw new Error('istek ApiError ile reddedilmeliydi')
  return err
}

describe('listScheduled', () => {
  it('sayfayı GET ile ister ve doğrulanmış yanıtı döndürür', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, pageBody({ page: 2, total: 11 })))
    const controller = new AbortController()

    const result = await listScheduled(2, controller.signal)

    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/scheduled?page=2')
    expect(new Headers(init?.headers).get('Accept')).toBe('application/json')
    expect(init?.signal).toBe(controller.signal)
    expect(result.total).toBe(11)
    expect(result.server_time).toBe('2026-11-21T09:00:00Z')
    expect(result.items[0]).toEqual(scheduledRow())
  })

  it('sunucu hatasını koduyla ApiError yapar', async () => {
    fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))

    const err = await rejection(listScheduled(1))

    expect(err.code).toBe('INTERNAL')
    expect(err.retryable).toBe(true)
  })

  it('geçersiz sayfa parametresi (400) yeniden denenemez', async () => {
    fetchMock.mockResolvedValue(errorResponse(400, 'INVALID_REQUEST'))

    expect((await rejection(listScheduled(0))).retryable).toBe(false)
  })

  it('ağ hatasını NETWORK yapar', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))

    expect((await rejection(listScheduled(1))).code).toBe('NETWORK')
  })

  it('iptali AbortError olarak olduğu gibi fırlatır', async () => {
    const abort = new DOMException('iptal', 'AbortError')
    fetchMock.mockRejectedValue(abort)

    await expect(listScheduled(1)).rejects.toBe(abort)
  })

  it('bozuk gövdeyi UNEXPECTED_RESPONSE yapar', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { items: 'x' }))

    expect((await rejection(listScheduled(1))).code).toBe('UNEXPECTED_RESPONSE')
  })
})

describe('parseScheduledPage', () => {
  it('boş sayfayı kabul eder', () => {
    expect(parseScheduledPage(200, pageBody({ items: [], total: 0 })).items).toEqual([])
  })

  const invalid: Record<string, unknown> = {
    'gövde yok': undefined,
    'items dizi değil': pageBody({ items: {} }),
    'toplam negatif': pageBody({ total: -1 }),
    'sayfa 0': pageBody({ page: 0 }),
    'limit 0': pageBody({ limit: 0 }),
    'limitten fazla kişi': pageBody({
      limit: 1,
      total: 2,
      items: [scheduledRow(), scheduledRow()],
    }),
    'toplamdan fazla': pageBody({ total: 0 }),
    'sunucu saati yok': pageBody({ server_time: undefined }),
    'sunucu saati bozuk': pageBody({ server_time: 'dün' }),
    'kimlik boş': pageBody({ items: [{ ...scheduledRow(), id: '' }] }),
    'brief sayı': pageBody({ items: [{ ...scheduledRow(), brief: 5 }] }),
    'planlanan zaman bozuk': pageBody({ items: [{ ...scheduledRow(), scheduled_at: 'yarın' }] }),
    'onay zamanı yok': pageBody({ items: [{ ...scheduledRow(), approved_at: undefined }] }),
    'kuyruk negatif': pageBody({ items: [{ ...scheduledRow(), queued: -1 }] }),
    'due metin': pageBody({ items: [{ ...scheduledRow(), due: 'evet' }] }),
    'due yok': pageBody({ items: [{ ...scheduledRow(), due: undefined }] }),
  }
  for (const [name, body] of Object.entries(invalid)) {
    it(`${name}: UNEXPECTED_RESPONSE`, () => {
      expect(() => parseScheduledPage(200, JSON.parse(JSON.stringify(body ?? null)))).toThrowError(
        ApiError,
      )
    })
  }
})
