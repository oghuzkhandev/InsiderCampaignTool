// Kampanyanın alıcı listesi API istemcisi: GET /api/campaigns/{id}/recipients. Gönderim takibindeki
// "Alıcılar" çekmecesi kullanır: kim, hangi kanaldan, hangi adrese ve ne oldu. Yanıtlar `unknown`
// olarak alınıp burada doğrulanır; bozuk bir yanıt `UNEXPECTED_RESPONSE` olur.

import {
  ApiError,
  invalidResponse,
  isAbortError,
  isCount,
  isRecord,
  readJson,
  toApiError,
} from './campaigns'
import { CHANNEL_KEYS, type ChannelKey } from './delivery'

/** Backend'deki sayfa boyutu üst sınırıyla uyumlu; çekmecede 10 alıcı gösterilir. */
export const RECIPIENT_PAGE_SIZE = 10

/** Backend'deki `q` üst sınırı (karakter). */
export const MAX_RECIPIENT_SEARCH_LENGTH = 100

export const MESSAGE_STATUSES = ['queued', 'sending', 'sent', 'failed', 'skipped'] as const
export type MessageStatus = (typeof MESSAGE_STATUSES)[number]

/** Süzgeç değeri: boş metin "süzme yok" demektir. */
export type ChannelFilter = ChannelKey | ''
export type StatusFilter = MessageStatus | ''

export interface RecipientFilters {
  /** Ad, e-posta veya telefonda aranan metin. */
  search: string
  channel: ChannelFilter
  status: StatusFilter
}

export const EMPTY_RECIPIENT_FILTERS: RecipientFilters = { search: '', channel: '', status: '' }

/** Kampanyanın bir mesajının alıcısı. Eksik ad `null`'dır (boş metin gelmez). */
export interface Recipient {
  /** Mesaj kimliği: aynı kişinin SMS ve e-postası ayrı satırlardır. */
  id: number
  user_id: number
  first_name: string | null
  channel: ChannelKey
  /** Onay anındaki telefon veya e-posta. */
  address: string
  status: MessageStatus
  attempts: number
  /** Gönderildiği an (yalnızca `sent`). */
  sent_at: string | null
  /** Başarısızlık nedeni kodu (yalnızca `failed`); bilinmiyorsa null. */
  failure_reason: string | null
}

export interface RecipientPage {
  page: number
  limit: number
  /** Süzgece uyan mesaj sayısı. */
  total: number
  items: Recipient[]
}

export function isMessageStatus(v: unknown): v is MessageStatus {
  return MESSAGE_STATUSES.some((status) => status === v)
}

function isChannel(v: unknown): v is ChannelKey {
  return CHANNEL_KEYS.some((key) => key === v)
}

function isTextOrNull(v: unknown): v is string | null {
  return v === null || typeof v === 'string'
}

function parseRecipient(status: number, row: unknown): Recipient {
  if (
    !isRecord(row) ||
    typeof row.id !== 'number' ||
    !Number.isSafeInteger(row.id) ||
    typeof row.user_id !== 'number' ||
    !Number.isSafeInteger(row.user_id) ||
    !isTextOrNull(row.first_name) ||
    !isChannel(row.channel) ||
    typeof row.address !== 'string' ||
    !isMessageStatus(row.status) ||
    !isCount(row.attempts) ||
    !isTextOrNull(row.sent_at) ||
    !isTextOrNull(row.failure_reason)
  ) {
    throw invalidResponse(status)
  }
  return {
    id: row.id,
    user_id: row.user_id,
    first_name: row.first_name,
    channel: row.channel,
    address: row.address,
    status: row.status,
    attempts: row.attempts,
    sent_at: row.sent_at,
    failure_reason: row.failure_reason,
  }
}

export function parseRecipientPage(status: number, body: unknown): RecipientPage {
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
    items: body.items.map((row) => parseRecipient(status, row)),
  }
}

/** Boş süzgeçler adrese yazılmaz. */
export function recipientsUrl(campaignId: string, f: RecipientFilters, page: number): string {
  const params = new URLSearchParams({ page: String(page), limit: String(RECIPIENT_PAGE_SIZE) })
  const search = f.search.trim()
  if (search) params.set('q', search)
  if (f.channel) params.set('channel', f.channel)
  if (f.status) params.set('status', f.status)
  return `/api/campaigns/${encodeURIComponent(campaignId)}/recipients?${params.toString()}`
}

/**
 * Kampanyanın alıcılarının `page`. sayfasını (1'den başlar) okur. Salt okunurdur. İptal edilirse
 * (AbortSignal) AbortError olduğu gibi fırlatılır.
 */
export async function listRecipients(
  campaignId: string,
  filters: RecipientFilters,
  page: number,
  signal?: AbortSignal,
): Promise<RecipientPage> {
  let response: Response
  try {
    response = await fetch(recipientsUrl(campaignId, filters, page), {
      headers: { Accept: 'application/json' },
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }
  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return parseRecipientPage(response.status, body)
}
