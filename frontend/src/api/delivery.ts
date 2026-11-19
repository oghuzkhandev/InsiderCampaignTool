// Gönderim takibi (dashboard) API istemcisi: GET /api/campaigns/{id}/events (SSE).
// Sözleşme: docs/ARCHITECTURE.md §5, davranış: docs/specs/TRK-001. Dışarıdan gelen
// her şey `unknown` olarak alınır ve uygulama tiplerine doğrulanarak çevrilir.
import {
  ApiError,
  invalidResponse,
  isAbortError,
  isCount,
  isRecord,
  readJson,
  toApiError,
} from './campaigns'
import { readSse } from './sse'

export const CAMPAIGN_STATUSES = ['draft', 'approved', 'sending', 'completed'] as const
export type CampaignStatus = (typeof CAMPAIGN_STATUSES)[number]

/**
 * Kampanyanın mesaj sayıları (TRK-001). `queued` yalnızca hiç denenmemiş,
 * `retrying` denenmiş ve yeniden sırada bekleyen mesajlardır; ikisi birbirine
 * dahil değildir.
 */
export interface DeliveryCounts {
  queued: number
  retrying: number
  sending: number
  sent: number
  failed: number
  skipped: number
}

/** Başarısız mesajların nedeni (`failure_reason`); toplamı `failed`'i aşamaz. */
export interface FailureReasonCounts {
  /** Deneme sınırı doldu; sonuç doğrulanamadı (kesin teslim edilmedi demek değildir). */
  exhausted: number
  /** Sağlayıcı isteği kesin olarak reddetti (429 dışında bir 4xx). */
  rejected: number
}

/** Her saniye (ve bağlantı kurulunca ilk olarak) gelen güncel durum görüntüsü. */
export interface DeliverySnapshot {
  status: CampaignStatus
  messages: DeliveryCounts
  failures: FailureReasonCounts
}

const eventsUrl = (id: string) => `/api/campaigns/${encodeURIComponent(id)}/events`

export function isCampaignStatus(v: unknown): v is CampaignStatus {
  return CAMPAIGN_STATUSES.some((status) => status === v)
}

/**
 * Kampanyanın gönderim durumunu SSE ile izler. Bağlantı her zaman güncel bir
 * `snapshot` ile başlar; sonraki her `snapshot` öncekinin yerini alır (üzerine
 * eklenmez). Akış hiçbir zaman "normal" bitmez: sunucu kapatsa da, ağ kopsa da,
 * `error` olayı gelse de ApiError fırlatılır; iptal AbortError olarak aynen
 * iletilir. Bağlantı kesilince yeniden bağlanmak çağıranın işidir.
 */
export async function streamDeliverySnapshots(
  id: string,
  onSnapshot: (snapshot: DeliverySnapshot) => void,
  signal?: AbortSignal,
): Promise<void> {
  let response: Response
  try {
    response = await fetch(eventsUrl(id), {
      headers: { Accept: 'text/event-stream' },
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }
  // Kampanya yoksa vb. durumlar akış başlamadan önce düz JSON hatadır.
  if (!response.ok) throw toApiError(response.status, await readJson(response))
  if (!response.body) throw invalidResponse(response.status)

  try {
    for await (const event of readSse(response.body)) {
      const data = parseJson(event.data)
      switch (event.event) {
        case 'snapshot':
          onSnapshot(parseSnapshot(response.status, data))
          break
        case 'error':
          throw errorFromEvent(response.status, data)
        default:
          break // bilinmeyen olaylar yok sayılır
      }
    }
  } catch (err) {
    if (err instanceof ApiError || isAbortError(err)) throw err
    // Akış ortasında bağlantı koptu.
    throw interrupted(response.status)
  }
  // Sunucu akışı kapattı: durum görüntüsü akışı sürmeliydi, yani yarıda kesildi.
  throw interrupted(response.status)
}

function interrupted(status: number): ApiError {
  return new ApiError(status, 'STREAM_INTERRUPTED', 'Gönderim takibi bağlantısı koptu.')
}

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

function parseCounts(data: unknown): DeliveryCounts | undefined {
  if (!isRecord(data)) return undefined
  const { queued, retrying, sending, sent, failed, skipped } = data
  if (
    !isCount(queued) ||
    !isCount(retrying) ||
    !isCount(sending) ||
    !isCount(sent) ||
    !isCount(failed) ||
    !isCount(skipped)
  ) {
    return undefined
  }
  return { queued, retrying, sending, sent, failed, skipped }
}

function parseFailures(data: unknown): FailureReasonCounts | undefined {
  if (!isRecord(data)) return undefined
  const { exhausted, rejected } = data
  if (!isCount(exhausted) || !isCount(rejected)) return undefined
  return { exhausted, rejected }
}

function parseSnapshot(status: number, data: unknown): DeliverySnapshot {
  if (!isRecord(data) || !isCampaignStatus(data.status)) throw invalidResponse(status)
  const messages = parseCounts(data.messages)
  const failures = parseFailures(data.failures)
  // Neden sayılarının toplamı başarısız sayısını aşarsa ekran tutarsız olurdu.
  if (!messages || !failures || failures.exhausted + failures.rejected > messages.failed) {
    throw invalidResponse(status)
  }
  return { status: data.status, messages, failures }
}

/** SSE `error` olayını ApiError'a çevirir (toApiError ile aynı hata biçimi). */
function errorFromEvent(status: number, data: unknown): ApiError {
  if (!isRecord(data) || typeof data.code !== 'string') return invalidResponse(status)
  return toApiError(status, { error: data })
}
