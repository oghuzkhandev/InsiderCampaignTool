import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AUDIENCE_PAGE_SIZE, listAudienceMembers, parseAudiencePage } from '../audience'
import { ApiError } from '../campaigns'
import { audiencePageBody, errorResponse, jsonResponse, memberBody } from '@/__tests__/helpers'

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

const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'

describe('listAudienceMembers', () => {
  it('sayfayı GET ile, sayfa ve limit parametreleriyle ister', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody({ page: 2, total: 23 })))
    const controller = new AbortController()

    const result = await listAudienceMembers(ID, 2, controller.signal)

    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe(`/api/campaigns/${ID}/audience?page=2&limit=10`)
    expect(init?.method).toBeUndefined() // GET
    expect(new Headers(init?.headers).get('Accept')).toBe('application/json')
    expect(init?.signal).toBe(controller.signal)
    expect(result.page).toBe(2)
    expect(result.total).toBe(23)
    expect(AUDIENCE_PAGE_SIZE).toBe(10)
  })

  it('kampanya kimliğini adres için kodlar', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody()))

    await listAudienceMembers('a/b?c', 1)

    expect(fetchMock.mock.calls[0]![0]).toBe('/api/campaigns/a%2Fb%3Fc/audience?page=1&limit=10')
  })

  it('eksik ad, e-posta ve telefonu null olarak korur', async () => {
    const body = audiencePageBody({
      items: [
        memberBody({
          first_name: null,
          email: null,
          phone: null,
          sms_status: 'no_consent',
          email_status: 'no_address',
        }),
      ],
    })
    fetchMock.mockResolvedValue(jsonResponse(200, body))

    const { items } = await listAudienceMembers(ID, 1)

    expect(items[0]).toEqual({
      id: 1,
      first_name: null,
      city: 'İstanbul',
      email: null,
      phone: null,
      sms_status: 'no_consent',
      email_status: 'no_address',
    })
  })

  it('son sayfanın ötesinde boş liste ve toplamı döndürür', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, audiencePageBody({ page: 9, total: 7, items: [] })),
    )

    const result = await listAudienceMembers(ID, 9)

    expect(result.items).toEqual([])
    expect(result.total).toBe(7)
  })

  it('sunucu hatasını koduyla ApiError yapar', async () => {
    fetchMock.mockResolvedValue(errorResponse(404, 'CAMPAIGN_NOT_FOUND'))

    const err = await rejection(listAudienceMembers(ID, 1))

    expect(err.status).toBe(404)
    expect(err.code).toBe('CAMPAIGN_NOT_FOUND')
    expect(err.retryable).toBe(false)
  })

  it('beklenmeyen sunucu hatası yeniden denenebilirdir', async () => {
    fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))

    const err = await rejection(listAudienceMembers(ID, 1))

    expect(err.code).toBe('INTERNAL')
    expect(err.retryable).toBe(true)
  })

  it('JSON olmayan hata gövdesini (ör. proxy 502) UNEXPECTED_RESPONSE yapar', async () => {
    fetchMock.mockResolvedValue(new Response('<html>Bad Gateway</html>', { status: 502 }))

    const err = await rejection(listAudienceMembers(ID, 1))

    expect(err.code).toBe('UNEXPECTED_RESPONSE')
  })

  it('ağ hatasını NETWORK yapar', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))

    const err = await rejection(listAudienceMembers(ID, 1))

    expect(err.code).toBe('NETWORK')
    expect(err.retryable).toBe(true)
  })

  it('iptali AbortError olarak olduğu gibi fırlatır', async () => {
    const abort = new DOMException('iptal', 'AbortError')
    fetchMock.mockRejectedValue(abort)

    await expect(listAudienceMembers(ID, 1)).rejects.toBe(abort)
  })

  it('geçersiz gövdeli başarılı yanıtı UNEXPECTED_RESPONSE yapar', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { page: 1 }))

    const err = await rejection(listAudienceMembers(ID, 1))

    expect(err.code).toBe('UNEXPECTED_RESPONSE')
  })
})

describe('parseAudiencePage', () => {
  const valid = () => audiencePageBody({ total: 3, items: [memberBody(), memberBody({ id: 2 })] })

  it('geçerli gövdeyi olduğu gibi döndürür', () => {
    expect(parseAudiencePage(200, valid())).toEqual(valid())
  })

  it('üç durumun hepsini tanır', () => {
    for (const status of ['eligible', 'no_consent', 'no_address'] as const) {
      const body = audiencePageBody({
        items: [memberBody({ sms_status: status, email_status: status })],
      })
      expect(parseAudiencePage(200, body).items[0]!.sms_status).toBe(status)
    }
  })

  const invalid: Record<string, unknown> = {
    'gövde yok': undefined,
    'gövde nesne değil': [],
    'sayfa yok': { ...valid(), page: undefined },
    'sayfa 0': { ...valid(), page: 0 },
    'sayfa ondalıklı': { ...valid(), page: 1.5 },
    'limit 0': { ...valid(), limit: 0 },
    'toplam negatif': { ...valid(), total: -1 },
    'toplam metin': { ...valid(), total: '3' },
    'items dizi değil': { ...valid(), items: {} },
    'items yok': { ...valid(), items: undefined },
    'limitten fazla kişi': { ...valid(), limit: 1 },
    'kişi nesne değil': { ...valid(), items: ['x'] },
    'kimlik metin': { ...valid(), items: [{ ...memberBody(), id: '1' }] },
    'kimlik ondalıklı': { ...valid(), items: [{ ...memberBody(), id: 1.5 }] },
    'şehir yok': { ...valid(), items: [{ ...memberBody(), city: undefined }] },
    'ad sayı': { ...valid(), items: [{ ...memberBody(), first_name: 5 }] },
    'ad alanı hiç yok': { ...valid(), items: [{ ...memberBody(), first_name: undefined }] },
    'e-posta sayı': { ...valid(), items: [{ ...memberBody(), email: 5 }] },
    'telefon nesne': { ...valid(), items: [{ ...memberBody(), phone: {} }] },
    'bilinmeyen sms durumu': { ...valid(), items: [{ ...memberBody(), sms_status: 'maybe' }] },
    'bilinmeyen e-posta durumu': { ...valid(), items: [{ ...memberBody(), email_status: 5 }] },
    'sms durumu yok': { ...valid(), items: [{ ...memberBody(), sms_status: undefined }] },
  }
  for (const [name, body] of Object.entries(invalid)) {
    it(`${name}: UNEXPECTED_RESPONSE`, () => {
      expect(() => parseAudiencePage(200, JSON.parse(JSON.stringify(body ?? null)))).toThrowError(
        ApiError,
      )
    })
  }
})
