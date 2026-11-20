// Kampanya önizleme API istemcisi (POST /api/campaigns). Sözleşme:
// docs/ARCHITECTURE.md §5.

/** Backend'deki segment.MaxBriefRunes ile aynı: Unicode karakter sayısı. */
export const MAX_BRIEF_LENGTH = 1000

export interface Condition {
  field: string
  op: string
  value: string | string[] | number
}

export interface Segment {
  all: Condition[]
}

export interface Audience {
  total: number
  sms: number
  email: number
}

export interface Campaign {
  id: string
  brief: string
  status: string
  segment: Segment
  audience: Audience
  created_at: string
}

export interface ApiReason {
  code: string
  field?: string
  text?: string
  /** Sebebin kaç alıcıyı etkilediği (ör. onayda render sınırı aşımı). */
  count?: number
}

/** İçerik alanları (SMS, e-posta konusu ve gövdesi). */
export interface ContentFields {
  sms_text: string
  email_subject: string
  email_body: string
}

/** Sunucudan gelen veya ağ kaynaklı bir hata. */
export class ApiError extends Error {
  status: number
  code: string
  reasons: ApiReason[]
  /** Doğrulanamayan üretimde çözümlenebilen, doğrulanmamış metinler. */
  partial?: ContentFields

  constructor(
    status: number,
    code: string,
    message: string,
    reasons: ApiReason[] = [],
    partial?: ContentFields,
  ) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.reasons = reasons
    this.partial = partial
  }

  /** Aynı brief'in yeniden denenmesi anlamlı mı? */
  get retryable(): boolean {
    return RETRYABLE_CODES.has(this.code)
  }
}

const RETRYABLE_CODES = new Set([
  'LLM_TIMEOUT',
  'STREAM_INTERRUPTED',
  'LLM_RATE_LIMITED',
  'INTERNAL',
  'NETWORK',
  'UNEXPECTED_RESPONSE',
])

const ENDPOINT = '/api/campaigns'

/**
 * Brief'ten segment üretir, kitle sayılarını hesaplar ve draft kampanya
 * oluşturur. İptal edilirse (AbortSignal) AbortError olduğu gibi fırlatılır.
 */
export async function createCampaign(brief: string, signal?: AbortSignal): Promise<Campaign> {
  let response: Response
  try {
    response = await fetch(ENDPOINT, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ brief }),
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }

  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return parseCampaign(response.status, body)
}

/** Listedeki bir kampanya satırı (GET /api/campaigns, CMP-001). Kitle sayısı ve segment yoktur. */
export interface CampaignSummary {
  scheduled_at?: string
  id: string
  brief: string
  status: string
  created_at: string
}

/** Backend'deki campaign.DefaultListLimit ve MaxListLimit ile aynı (CMP-001). */
export const DEFAULT_LIST_LIMIT = 10
export const MAX_LIST_LIMIT = 20

/**
 * En yeni kampanyaları (en yeni başta) listeler. `limit` verilmezse 10, en fazla 20 olur;
 * daha büyük değeri sunucu 20'ye indirir. İptal edilirse (AbortSignal) AbortError olduğu gibi
 * fırlatılır.
 */
export async function listCampaigns(
  limit: number = DEFAULT_LIST_LIMIT,
  signal?: AbortSignal,
): Promise<CampaignSummary[]> {
  let response: Response
  try {
    response = await fetch(`${ENDPOINT}?limit=${encodeURIComponent(String(limit))}`, {
      headers: { Accept: 'application/json' },
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }

  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return parseCampaignList(response.status, body)
}

export function parseCampaignList(status: number, body: unknown): CampaignSummary[] {
  if (!isRecord(body) || !Array.isArray(body.campaigns)) throw invalidResponse(status)
  const list: CampaignSummary[] = []
  for (const row of body.campaigns) {
    if (
      !isRecord(row) ||
      typeof row.id !== 'string' ||
      typeof row.brief !== 'string' ||
      typeof row.status !== 'string' ||
      typeof row.created_at !== 'string' ||
      Number.isNaN(Date.parse(row.created_at))
    ) {
      throw invalidResponse(status)
    }
    if (
      row.scheduled_at !== undefined &&
      (typeof row.scheduled_at !== 'string' || Number.isNaN(Date.parse(row.scheduled_at)))
    )
      throw invalidResponse(status)
    list.push({
      id: row.id,
      brief: row.brief,
      status: row.status,
      created_at: row.created_at,
      ...(typeof row.scheduled_at === 'string' ? { scheduled_at: row.scheduled_at } : {}),
    })
  }
  return list
}

export function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === 'AbortError'
}

export async function readJson(response: Response): Promise<unknown> {
  try {
    return await response.json()
  } catch {
    return undefined
  }
}

export function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

export function toApiError(status: number, body: unknown): ApiError {
  const error = isRecord(body) ? body.error : undefined
  if (!isRecord(error) || typeof error.code !== 'string') {
    // Ör. API kapalıyken proxy'nin döndürdüğü 502 veya HTML sayfası.
    return new ApiError(status, 'UNEXPECTED_RESPONSE', 'Sunucudan beklenmeyen bir yanıt alındı.')
  }
  const reasons: ApiReason[] = []
  if (Array.isArray(error.reasons)) {
    for (const r of error.reasons) {
      if (isRecord(r) && typeof r.code === 'string') {
        reasons.push({
          code: r.code,
          field: typeof r.field === 'string' ? r.field : undefined,
          text: typeof r.text === 'string' ? r.text : undefined,
          count: Number.isInteger(r.count) ? (r.count as number) : undefined,
        })
      }
    }
  }
  const message = typeof error.message === 'string' ? error.message : ''
  return new ApiError(status, error.code, message, reasons)
}

export function invalidResponse(status: number): ApiError {
  return new ApiError(status, 'UNEXPECTED_RESPONSE', 'Sunucudan beklenmeyen bir yanıt alındı.')
}

export function parseCampaign(status: number, body: unknown): Campaign {
  if (!isRecord(body)) throw invalidResponse(status)
  const { id, brief, status: campaignStatus, segment, audience, created_at } = body
  if (
    typeof id !== 'string' ||
    typeof brief !== 'string' ||
    typeof campaignStatus !== 'string' ||
    typeof created_at !== 'string' ||
    !isRecord(audience) ||
    !isCount(audience.total) ||
    !isCount(audience.sms) ||
    !isCount(audience.email) ||
    !isRecord(segment) ||
    !Array.isArray(segment.all)
  ) {
    throw invalidResponse(status)
  }

  const all: Condition[] = []
  for (const c of segment.all) {
    if (!isRecord(c) || typeof c.field !== 'string' || typeof c.op !== 'string') {
      throw invalidResponse(status)
    }
    const value = c.value
    const ok =
      typeof value === 'string' ||
      typeof value === 'number' ||
      (Array.isArray(value) && value.every((v) => typeof v === 'string'))
    if (!ok) throw invalidResponse(status)
    all.push({ field: c.field, op: c.op, value: value as Condition['value'] })
  }

  return {
    id,
    brief,
    status: campaignStatus,
    segment: { all },
    audience: { total: audience.total, sms: audience.sms, email: audience.email },
    created_at,
  }
}

export function isCount(v: unknown): v is number {
  return typeof v === 'number' && Number.isInteger(v) && v >= 0
}
