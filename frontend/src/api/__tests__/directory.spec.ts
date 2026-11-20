import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  EMPTY_FILTERS,
  activeFilterCount,
  directoryUrl,
  listCities,
  listDirectory,
  parseCities,
  type DirectoryFilters,
} from '../directory'
import { ApiError } from '../campaigns'
import { audiencePageBody, errorResponse, jsonResponse } from '@/__tests__/helpers'

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

const filters = (over: Partial<DirectoryFilters>): DirectoryFilters => ({
  ...EMPTY_FILTERS,
  ...over,
})

describe('directoryUrl', () => {
  it('süzgeç yokken yalnızca sayfa ve limit yazar', () => {
    expect(directoryUrl(EMPTY_FILTERS, 1)).toBe('/api/audience?page=1&limit=10')
  })

  it('dolu süzgeçleri adrese yazar, boşları yazmaz', () => {
    const url = directoryUrl(filters({ city: 'İzmir', sms: 'eligible' }), 3)

    const params = new URL(url, 'http://x').searchParams
    expect(params.get('page')).toBe('3')
    expect(params.get('city')).toBe('İzmir')
    expect(params.get('sms')).toBe('eligible')
    expect(params.has('email')).toBe(false)
    expect(params.has('search')).toBe(false)
  })

  it('aramayı kırpar, kodlar ve başka parametreye dönüştürmez', () => {
    const url = directoryUrl(filters({ search: '  ayşe&limit=100  ' }), 1)

    const params = new URL(url, 'http://x').searchParams
    expect(params.get('search')).toBe('ayşe&limit=100')
    expect(params.get('limit')).toBe('10') // arama içindeki "&limit" parametre olmadı
  })

  it('yalnızca boşluktan oluşan aramayı yazmaz', () => {
    expect(directoryUrl(filters({ search: '   ' }), 1)).toBe('/api/audience?page=1&limit=10')
  })
})

describe('activeFilterCount', () => {
  it('süzgeç yokken 0', () => {
    expect(activeFilterCount(EMPTY_FILTERS)).toBe(0)
  })

  it('her dolu süzgeci sayar; boşluk araması sayılmaz', () => {
    expect(activeFilterCount(filters({ search: 'a', city: 'Adana' }))).toBe(2)
    expect(
      activeFilterCount(
        filters({ search: 'a', city: 'Adana', sms: 'no_consent', email: 'eligible' }),
      ),
    ).toBe(4)
    expect(activeFilterCount(filters({ search: '  ' }))).toBe(0)
  })
})

describe('listDirectory', () => {
  it('süzgeçli adresi GET ile ister ve sayfayı döndürür', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, audiencePageBody({ page: 2, total: 14 })))
    const controller = new AbortController()

    const result = await listDirectory(filters({ city: 'Adana' }), 2, controller.signal)

    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/audience?page=2&limit=10&city=Adana')
    expect(new Headers(init?.headers).get('Accept')).toBe('application/json')
    expect(init?.signal).toBe(controller.signal)
    expect(result.total).toBe(14)
  })

  it('sunucu hatasını koduyla ApiError yapar (geçersiz süzgeç 400)', async () => {
    fetchMock.mockResolvedValue(errorResponse(400, 'INVALID_REQUEST'))

    const err = await rejection(listDirectory(EMPTY_FILTERS, 1))

    expect(err.status).toBe(400)
    expect(err.code).toBe('INVALID_REQUEST')
    expect(err.retryable).toBe(false)
  })

  it('ağ hatasını NETWORK yapar', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))

    expect((await rejection(listDirectory(EMPTY_FILTERS, 1))).code).toBe('NETWORK')
  })

  it('iptali AbortError olarak olduğu gibi fırlatır', async () => {
    const abort = new DOMException('iptal', 'AbortError')
    fetchMock.mockRejectedValue(abort)

    await expect(listDirectory(EMPTY_FILTERS, 1)).rejects.toBe(abort)
  })

  it('bozuk gövdeyi UNEXPECTED_RESPONSE yapar', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { items: 'x' }))

    expect((await rejection(listDirectory(EMPTY_FILTERS, 1))).code).toBe('UNEXPECTED_RESPONSE')
  })
})

describe('listCities', () => {
  it('şehirleri Türkçe alfabe sırasıyla döndürür', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse(200, { cities: ['İzmir', 'Ankara', 'Çanakkale', 'Adana', 'İstanbul', 'Bursa'] }),
    )

    expect(await listCities()).toEqual([
      'Adana',
      'Ankara',
      'Bursa',
      'Çanakkale',
      'İstanbul',
      'İzmir',
    ])
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/audience/cities')
  })

  it('boş listeyi kabul eder', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { cities: [] }))

    expect(await listCities()).toEqual([])
  })

  it('sunucu hatasını ApiError yapar', async () => {
    fetchMock.mockResolvedValue(errorResponse(500, 'INTERNAL'))

    expect((await rejection(listCities())).code).toBe('INTERNAL')
  })
})

describe('parseCities', () => {
  it.each([
    ['gövde yok', undefined],
    ['cities dizi değil', { cities: 'Adana' }],
    ['cities yok', {}],
    ['metin olmayan öğe', { cities: ['Adana', 5] }],
    ['boş şehir adı', { cities: ['Adana', ''] }],
  ])('%s: UNEXPECTED_RESPONSE', (_name, body) => {
    expect(() => parseCities(200, body)).toThrowError(ApiError)
  })
})
