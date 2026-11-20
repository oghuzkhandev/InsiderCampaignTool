// Kampanyadan bağımsız kişi dizini API istemcisi: GET /api/audience (arama, şehir, SMS ve e-posta
// uygunluğu süzgeçleri, sayfalı) ve GET /api/audience/cities (şehir seçenekleri). Yanıtlar
// `unknown` olarak alınıp burada doğrulanır; bozuk bir yanıt `UNEXPECTED_RESPONSE` olur.

import {
  AUDIENCE_PAGE_SIZE,
  parseAudiencePage,
  type AudiencePage,
  type ChannelStatus,
} from './audience'
import {
  ApiError,
  invalidResponse,
  isAbortError,
  isRecord,
  readJson,
  toApiError,
} from './campaigns'

/** Süzgeç değeri: boş metin "süzme yok" demektir. */
export type StatusFilter = ChannelStatus | ''

/** Dizin süzgeçleri. Boş alan süzmez; dolu alanlar AND ile birleşir. */
export interface DirectoryFilters {
  /** Ad, e-posta veya telefonda aranan metin (şehri kapsamaz). */
  search: string
  /** Şehir (tam eşleşme). */
  city: string
  sms: StatusFilter
  email: StatusFilter
}

export const EMPTY_FILTERS: DirectoryFilters = { search: '', city: '', sms: '', email: '' }

/** Backend'deki `search` üst sınırı (karakter). */
export const MAX_SEARCH_LENGTH = 100

/** Dolu süzgeç sayısı ("Filtreleri temizle" ve rozet için). */
export function activeFilterCount(f: DirectoryFilters): number {
  return [f.search.trim(), f.city, f.sms, f.email].filter((v) => v !== '').length
}

/** Boş süzgeçler adrese yazılmaz. */
export function directoryUrl(f: DirectoryFilters, page: number): string {
  const params = new URLSearchParams({ page: String(page), limit: String(AUDIENCE_PAGE_SIZE) })
  const search = f.search.trim()
  if (search) params.set('search', search)
  if (f.city) params.set('city', f.city)
  if (f.sms) params.set('sms', f.sms)
  if (f.email) params.set('email', f.email)
  return `/api/audience?${params.toString()}`
}

async function get(url: string, signal?: AbortSignal): Promise<{ status: number; body: unknown }> {
  let response: Response
  try {
    response = await fetch(url, { headers: { Accept: 'application/json' }, signal })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }
  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return { status: response.status, body }
}

/** Süzgeçlere uyan kişilerin `page`. sayfası (1'den başlar); kimliğe göre sıralıdır. */
export async function listDirectory(
  filters: DirectoryFilters,
  page: number,
  signal?: AbortSignal,
): Promise<AudiencePage> {
  const { status, body } = await get(directoryUrl(filters, page), signal)
  return parseAudiencePage(status, body)
}

export function parseCities(status: number, body: unknown): string[] {
  if (!isRecord(body) || !Array.isArray(body.cities)) throw invalidResponse(status)
  if (!body.cities.every((c) => typeof c === 'string' && c !== '')) throw invalidResponse(status)
  return body.cities as string[]
}

/** Şehir süzgecinin seçenekleri: kullanıcıların farklı şehirleri, Türkçe sıralı. */
export async function listCities(signal?: AbortSignal): Promise<string[]> {
  const { status, body } = await get('/api/audience/cities', signal)
  return [...parseCities(status, body)].sort((a, b) => a.localeCompare(b, 'tr'))
}
