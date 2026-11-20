import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../campaigns'
import {
  EMPTY_RECIPIENT_FILTERS,
  listRecipients,
  parseRecipientPage,
  recipientsUrl,
} from '../recipients'
import { errorResponse, jsonResponse } from '@/__tests__/helpers'
import { recipientBody, recipientPageBody } from '@/__tests__/recipient-helpers'

const ID = '498882b6-2221-4589-a99f-67ceb68c59c2'
const fetchMock = vi.fn<typeof fetch>()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => vi.unstubAllGlobals())

describe('recipientsUrl', () => {
  it('süzgeçsiz istekte yalnızca sayfa ve limit yazılır', () => {
    expect(recipientsUrl(ID, EMPTY_RECIPIENT_FILTERS, 1)).toBe(
      `/api/campaigns/${ID}/recipients?page=1&limit=10`,
    )
  })

  it('dolu süzgeçler adrese yazılır; arama kırpılır ve kodlanır', () => {
    const url = recipientsUrl(ID, { search: '  ayşe & x ', channel: 'sms', status: 'failed' }, 3)

    const params = new URL(url, 'http://x').searchParams
    expect(Object.fromEntries(params)).toEqual({
      page: '3',
      limit: '10',
      q: 'ayşe & x',
      channel: 'sms',
      status: 'failed',
    })
  })

  it('kampanya kimliği adreste kodlanır', () => {
    expect(recipientsUrl('a/b', EMPTY_RECIPIENT_FILTERS, 1)).toContain('/api/campaigns/a%2Fb/')
  })
})

describe('parseRecipientPage', () => {
  it('geçerli yanıtı olduğu gibi çevirir (eksik değerler null kalır)', () => {
    const row = recipientBody({ first_name: null, sent_at: null, status: 'queued', attempts: 0 })

    const page = parseRecipientPage(200, recipientPageBody({ items: [row], total: 41 }))

    expect(page.total).toBe(41)
    expect(page.items).toEqual([row])
  })

  it.each([
    ['gövde nesne değil', 'x'],
    ['total eksik', { page: 1, limit: 10, items: [] }],
    ['page 0', { page: 0, limit: 10, total: 0, items: [] }],
    ['items dizi değil', { page: 1, limit: 10, total: 0, items: {} }],
    [
      'limitten çok satır',
      { page: 1, limit: 1, total: 2, items: [recipientBody(), recipientBody()] },
    ],
    [
      'tanınmayan durum',
      { page: 1, limit: 10, total: 1, items: [{ ...recipientBody(), status: 'done' }] },
    ],
    [
      'tanınmayan kanal',
      { page: 1, limit: 10, total: 1, items: [{ ...recipientBody(), channel: 'push' }] },
    ],
    [
      'adres metin değil',
      { page: 1, limit: 10, total: 1, items: [{ ...recipientBody(), address: 5 }] },
    ],
    ['ad sayı', { page: 1, limit: 10, total: 1, items: [{ ...recipientBody(), first_name: 7 }] }],
    [
      'deneme negatif',
      { page: 1, limit: 10, total: 1, items: [{ ...recipientBody(), attempts: -1 }] },
    ],
  ])('bozuk yanıt reddedilir: %s', (_name, body) => {
    expect(() => parseRecipientPage(200, body)).toThrowError(ApiError)
    expect(() => parseRecipientPage(200, body)).toThrowError(
      expect.objectContaining({ code: 'UNEXPECTED_RESPONSE' }),
    )
  })
})

describe('listRecipients', () => {
  it('doğru adresi ister ve sayfayı döndürür', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, recipientPageBody({ total: 1 })))

    const page = await listRecipients(ID, { search: '', channel: 'email', status: '' }, 2)

    expect(String(fetchMock.mock.calls[0]![0])).toBe(
      `/api/campaigns/${ID}/recipients?page=2&limit=10&channel=email`,
    )
    expect(page.items).toHaveLength(1)
  })

  it('sunucu hatasını ApiError olarak fırlatır', async () => {
    fetchMock.mockResolvedValue(errorResponse(404, 'CAMPAIGN_NOT_FOUND'))

    await expect(listRecipients(ID, EMPTY_RECIPIENT_FILTERS, 1)).rejects.toMatchObject({
      status: 404,
      code: 'CAMPAIGN_NOT_FOUND',
    })
  })

  it('ağ hatası NETWORK olur, iptal AbortError olarak kalır', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('failed'))
    await expect(listRecipients(ID, EMPTY_RECIPIENT_FILTERS, 1)).rejects.toMatchObject({
      code: 'NETWORK',
    })

    fetchMock.mockRejectedValueOnce(new DOMException('abort', 'AbortError'))
    await expect(listRecipients(ID, EMPTY_RECIPIENT_FILTERS, 1)).rejects.toMatchObject({
      name: 'AbortError',
    })
  })
})
