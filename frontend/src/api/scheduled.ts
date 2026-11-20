// Zamanlanmış gönderimler API istemcisi (GET /api/scheduled): onaylanmış ve gönderimi ileri bir
// zamana planlanmış, henüz başlamamış kampanyalar. Yanıt `unknown` olarak alınıp burada doğrulanır;
// bozuk bir yanıt `UNEXPECTED_RESPONSE` olur.

import {
  ApiError,
  invalidResponse,
  isAbortError,
  isCount,
  isRecord,
  readJson,
  toApiError,
} from './campaigns'

/** Backend'deki varsayılan sayfa boyutu (limit verilmezse 10). */
export const SCHEDULED_PAGE_SIZE = 10

/** Gönderimi planlanmış, henüz başlamamış bir kampanya. */
export interface ScheduledCampaign {
  id: string
  brief: string
  /** Planlanan başlangıç (UTC, RFC 3339). Gönderimin başlayabileceği EN ERKEN andır. */
  scheduled_at: string
  approved_at: string
  /** Gönderilmeyi bekleyen mesaj sayısı. */
  queued: number
  /** Planlanan zaman geldi ama gönderim başlamadı (ör. worker kapalı); iş kaybolmaz. */
  due: boolean
}

export interface ScheduledPage {
  items: ScheduledCampaign[]
  total: number
  page: number
  limit: number
  /** Sayfanın okunduğu andaki sunucu saati: kalan süre buna göre hesaplanır, cihaz saatine değil. */
  server_time: string
}

function isDateText(v: unknown): v is string {
  return typeof v === 'string' && !Number.isNaN(Date.parse(v))
}

function parseCampaign(status: number, row: unknown): ScheduledCampaign {
  if (
    !isRecord(row) ||
    typeof row.id !== 'string' ||
    row.id === '' ||
    typeof row.brief !== 'string' ||
    !isDateText(row.scheduled_at) ||
    !isDateText(row.approved_at) ||
    !isCount(row.queued) ||
    typeof row.due !== 'boolean'
  ) {
    throw invalidResponse(status)
  }
  return {
    id: row.id,
    brief: row.brief,
    scheduled_at: row.scheduled_at,
    approved_at: row.approved_at,
    queued: row.queued,
    due: row.due,
  }
}

export function parseScheduledPage(status: number, body: unknown): ScheduledPage {
  if (
    !isRecord(body) ||
    !Array.isArray(body.items) ||
    !isCount(body.total) ||
    !isCount(body.page) ||
    body.page < 1 ||
    !isCount(body.limit) ||
    body.limit < 1 ||
    body.items.length > body.limit ||
    body.items.length > body.total ||
    !isDateText(body.server_time)
  ) {
    throw invalidResponse(status)
  }
  return {
    items: body.items.map((row) => parseCampaign(status, row)),
    total: body.total,
    page: body.page,
    limit: body.limit,
    server_time: body.server_time,
  }
}

/** Planlı kampanyaların `page`. sayfası (1'den başlar), en yakın zaman başta. Salt okunurdur. */
export async function listScheduled(page: number, signal?: AbortSignal): Promise<ScheduledPage> {
  let response: Response
  try {
    response = await fetch(`/api/scheduled?page=${encodeURIComponent(String(page))}`, {
      headers: { Accept: 'application/json' },
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }
  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return parseScheduledPage(response.status, body)
}
