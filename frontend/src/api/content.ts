// İçerik üretimi, düzenleme ve onay API istemcisi. Sözleşme:
// docs/ARCHITECTURE.md §5.
import {
  ApiError,
  invalidResponse,
  isAbortError,
  isCount,
  isRecord,
  parseCampaign,
  readJson,
  toApiError,
  type ApiReason,
  type Campaign,
  type ContentFields,
} from './campaigns'
import { readSse } from './sse'

export type { ContentFields } from './campaigns'

export const CONTENT_FIELDS = ['sms_text', 'email_subject', 'email_body'] as const
export type ContentField = (typeof CONTENT_FIELDS)[number]

export interface ContentVersion extends ContentFields {
  content_version: number
  source: 'llm' | 'human'
  created_at: string
}

/** Kampanya ve varsa son doğrulanmış içerik ile onay bilgisi. */
export interface CampaignDetail extends Campaign {
  content?: ContentVersion
  approved_content_version?: number
  approved_at?: string
}

/** Akış sırasında gelen geçici önizleme parçası. */
export interface Delta {
  field: ContentField
  text: string
  /** true ise alanın önceki önizlemesi silinir ve text ile başlanır. */
  replace: boolean
}

export interface ChannelCounts {
  sms: number
  email: number
  total: number
}

export interface ApproveResult {
  id: string
  status: string
  approved_content_version: number
  approved_at: string
  queued: ChannelCounts
}

const ENDPOINT = '/api/campaigns'
const contentUrl = (id: string) => `${ENDPOINT}/${encodeURIComponent(id)}/content`

async function send(
  url: string,
  init: RequestInit,
): Promise<{ response: Response; body: unknown }> {
  let response: Response
  try {
    response = await fetch(url, init)
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }
  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return { response, body }
}

/** Kampanyayı, varsa son doğrulanmış içeriği ve onay bilgisini okur. */
export async function getCampaign(id: string, signal?: AbortSignal): Promise<CampaignDetail> {
  const { response, body } = await send(`${ENDPOINT}/${encodeURIComponent(id)}`, {
    headers: { Accept: 'application/json' },
    signal,
  })
  const campaign = parseCampaign(response.status, body)
  const detail: CampaignDetail = { ...campaign }
  if (isRecord(body)) {
    if (body.content !== undefined)
      detail.content = parseContentVersion(response.status, body.content)
    if (body.approved_content_version !== undefined) {
      if (!isCount(body.approved_content_version) || typeof body.approved_at !== 'string') {
        throw invalidResponse(response.status)
      }
      detail.approved_content_version = body.approved_content_version
      detail.approved_at = body.approved_at
    }
  }
  return detail
}

/** İnsan düzenlemesini yeni bir içerik sürümü olarak kaydeder (PUT). */
export async function saveContent(
  id: string,
  fields: ContentFields,
  signal?: AbortSignal,
): Promise<ContentVersion> {
  const { response, body } = await send(contentUrl(id), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({
      sms_text: fields.sms_text,
      email_subject: fields.email_subject,
      email_body: fields.email_body,
    }),
    signal,
  })
  return parseContentVersion(response.status, body)
}

/**
 * Kampanya içeriğini üretir ve SSE ile akıtır. Her geçici parça onDelta ile
 * verilir. Tamamı doğrulanıp kaydedilince (validated) sürüm döner. `error`
 * olayı, akışın tamamlanmadan bitmesi veya kopması ApiError olarak fırlatılır;
 * doğrulanamayan üretimde ApiError.partial çözümlenebilen metinleri taşır.
 */
export async function generateContent(
  id: string,
  onDelta: (delta: Delta) => void,
  signal?: AbortSignal,
): Promise<ContentVersion> {
  let response: Response
  try {
    response = await fetch(contentUrl(id), {
      method: 'POST',
      headers: { Accept: 'text/event-stream' },
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw err
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }
  if (!response.ok) throw toApiError(response.status, await readJson(response))
  if (!response.body) throw invalidResponse(response.status)

  try {
    for await (const event of readSse(response.body)) {
      const data = parseJson(event.data)
      switch (event.event) {
        case 'delta':
          onDelta(parseDelta(response.status, data))
          break
        case 'validated':
          return parseContentVersion(response.status, data)
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
  // Ne validated ne error geldi: akış yarıda kesildi.
  throw interrupted(response.status)
}

/** Kampanyayı verilen içerik sürümüyle onaylar; mesaj işleri aynı anda yazılır. */
export async function approveCampaign(
  id: string,
  contentVersion: number,
  signal?: AbortSignal,
): Promise<ApproveResult> {
  const { response, body } = await send(`${ENDPOINT}/${encodeURIComponent(id)}/approve`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ content_version: contentVersion }),
    signal,
  })
  return parseApproveResult(response.status, body)
}

function interrupted(status: number): ApiError {
  return new ApiError(status, 'STREAM_INTERRUPTED', 'İçerik üretimi yarıda kesildi.')
}

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

function isContentField(v: unknown): v is ContentField {
  return typeof v === 'string' && (CONTENT_FIELDS as readonly string[]).includes(v)
}

function parseDelta(status: number, data: unknown): Delta {
  if (!isRecord(data) || !isContentField(data.field) || typeof data.text !== 'string') {
    throw invalidResponse(status)
  }
  return { field: data.field, text: data.text, replace: data.replace === true }
}

function parseFields(data: Record<string, unknown>): ContentFields | undefined {
  const { sms_text, email_subject, email_body } = data
  if (
    typeof sms_text !== 'string' ||
    typeof email_subject !== 'string' ||
    typeof email_body !== 'string'
  ) {
    return undefined
  }
  return { sms_text, email_subject, email_body }
}

function parseContentVersion(status: number, data: unknown): ContentVersion {
  if (!isRecord(data)) throw invalidResponse(status)
  const fields = parseFields(data)
  if (
    !fields ||
    !isCount(data.content_version) ||
    data.content_version < 1 ||
    (data.source !== 'llm' && data.source !== 'human') ||
    typeof data.created_at !== 'string'
  ) {
    throw invalidResponse(status)
  }
  return {
    ...fields,
    content_version: data.content_version,
    source: data.source,
    created_at: data.created_at,
  }
}

function parseApproveResult(status: number, data: unknown): ApproveResult {
  if (
    !isRecord(data) ||
    typeof data.id !== 'string' ||
    typeof data.status !== 'string' ||
    !isCount(data.approved_content_version) ||
    typeof data.approved_at !== 'string' ||
    !isRecord(data.queued) ||
    !isCount(data.queued.sms) ||
    !isCount(data.queued.email) ||
    !isCount(data.queued.total)
  ) {
    throw invalidResponse(status)
  }
  return {
    id: data.id,
    status: data.status,
    approved_content_version: data.approved_content_version,
    approved_at: data.approved_at,
    queued: { sms: data.queued.sms, email: data.queued.email, total: data.queued.total },
  }
}

/** SSE `error` olayını ApiError'a çevirir (toApiError ile aynı sebep biçimi). */
function errorFromEvent(status: number, data: unknown): ApiError {
  if (!isRecord(data) || typeof data.code !== 'string') return invalidResponse(status)
  const base = toApiError(status, { error: data })
  const partial = isRecord(data.partial) ? parseFields(data.partial) : undefined
  const reasons: ApiReason[] = base.reasons
  return new ApiError(status, base.code, base.message, reasons, partial)
}
