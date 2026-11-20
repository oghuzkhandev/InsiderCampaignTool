// "Segmente uyan kişiler" listesi API istemcisi (GET /api/campaigns/{id}/audience, ADR-031).
// Sözleşme: docs/ARCHITECTURE.md §5. Dışarıdan gelen her yanıt `unknown` olarak alınıp burada
// doğrulanır; tanınmayan bir durum veya bozuk alan `UNEXPECTED_RESPONSE` olur.

import {
  ApiError,
  invalidResponse,
  isAbortError,
  isCount,
  isRecord,
  readJson,
  toApiError,
} from './campaigns'

/** Backend'deki audience.DefaultMemberLimit ile aynı: ilk sayfada 10 kişi (ADR-031). */
export const AUDIENCE_PAGE_SIZE = 10

/**
 * Bir kişinin bir kanala uygunluğu. Uygun olmak için açık izin VE adres gerekir (ADR-023);
 * izin yoksa adresin olup olmadığına bakılmadan `no_consent` gelir.
 */
export type ChannelStatus = 'eligible' | 'no_consent' | 'no_address'

const CHANNEL_STATUSES: readonly ChannelStatus[] = ['eligible', 'no_consent', 'no_address']

function isChannelStatus(v: unknown): v is ChannelStatus {
  return typeof v === 'string' && (CHANNEL_STATUSES as readonly string[]).includes(v)
}

/** Segmente uyan bir kişi. Eksik ad, e-posta veya telefon `null`'dır (boş metin gelmez). */
export interface AudienceMember {
  id: number
  first_name: string | null
  city: string
  email: string | null
  phone: string | null
  sms_status: ChannelStatus
  email_status: ChannelStatus
}

/** Listenin bir sayfası. `total`, kanal iznine bakılmadan eşleşen kişi sayısıdır. */
export interface AudiencePage {
  page: number
  limit: number
  total: number
  items: AudienceMember[]
}

/** Metin veya null; başka her şey (sayı, boş değer, nesne) geçersizdir. */
function isTextOrNull(v: unknown): v is string | null {
  return v === null || typeof v === 'string'
}

function parseMember(status: number, row: unknown): AudienceMember {
  if (
    !isRecord(row) ||
    typeof row.id !== 'number' ||
    !Number.isSafeInteger(row.id) ||
    typeof row.city !== 'string' ||
    !isTextOrNull(row.first_name) ||
    !isTextOrNull(row.email) ||
    !isTextOrNull(row.phone) ||
    !isChannelStatus(row.sms_status) ||
    !isChannelStatus(row.email_status)
  ) {
    throw invalidResponse(status)
  }
  return {
    id: row.id,
    first_name: row.first_name,
    city: row.city,
    email: row.email,
    phone: row.phone,
    sms_status: row.sms_status,
    email_status: row.email_status,
  }
}

export function parseAudiencePage(status: number, body: unknown): AudiencePage {
  if (
    !isRecord(body) ||
    !isCount(body.page) ||
    body.page < 1 ||
    !isCount(body.limit) ||
    body.limit < 1 ||
    !isCount(body.total) ||
    !Array.isArray(body.items) ||
    body.items.length > body.limit
  ) {
    throw invalidResponse(status)
  }
  return {
    page: body.page,
    limit: body.limit,
    total: body.total,
    items: body.items.map((row) => parseMember(status, row)),
  }
}

/**
 * Kampanyanın segmentine uyan kişilerin `page`. sayfasını (1'den başlar) okur. Salt okunurdur:
 * model çağrılmaz, hiçbir şey yazılmaz. İptal edilirse (AbortSignal) AbortError olduğu gibi
 * fırlatılır.
 */
export async function listAudienceMembers(
  campaignId: string,
  page: number,
  signal?: AbortSignal,
): Promise<AudiencePage> {
  const url =
    `/api/campaigns/${encodeURIComponent(campaignId)}/audience` +
    `?page=${encodeURIComponent(String(page))}&limit=${AUDIENCE_PAGE_SIZE}`

  let response: Response
  try {
    response = await fetch(url, { headers: { Accept: 'application/json' }, signal })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }

  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return parseAudiencePage(response.status, body)
}
