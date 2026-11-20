import {
  ApiError,
  invalidResponse,
  isCount,
  isRecord,
  parseCampaignList,
  readJson,
  toApiError,
  type CampaignSummary,
  type ContentFields,
} from './campaigns'

export interface Page<T> {
  items: T[]
  total: number
  page: number
  limit: number
}
export type ChannelStatus = 'eligible' | 'no_consent' | 'no_address'
export interface Member {
  id: number
  first_name: string | null
  city: string
  email: string | null
  phone: string | null
  sms_status: ChannelStatus
  email_status: ChannelStatus
}
export interface ContentTemplate extends ContentFields {
  id: string
  name: string
  created_at: string
  updated_at: string
}
export interface Overview {
  campaigns: number
  draft: number
  approved: number
  sending: number
  completed: number
  users: number
  sent: number
  failed: number
  pending: number
  recovered: number
  templates: number
}

async function request(
  path: string,
  signal?: AbortSignal,
  init: RequestInit = {},
): Promise<{ status: number; body: unknown }> {
  let response: Response
  try {
    response = await fetch(path, {
      ...init,
      headers: {
        Accept: 'application/json',
        ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      },
      signal,
    })
  } catch (error) {
    if (signal?.aborted || (error instanceof DOMException && error.name === 'AbortError'))
      throw error
    throw new ApiError(0, 'NETWORK', 'Sunucuya ulaşılamadı.')
  }
  const body = await readJson(response)
  if (!response.ok) throw toApiError(response.status, body)
  return { status: response.status, body }
}

function parsePage<T>(status: number, body: unknown, parse: (row: unknown) => T): Page<T> {
  if (
    !isRecord(body) ||
    !isCount(body.total) ||
    !isCount(body.page) ||
    body.page < 1 ||
    !isCount(body.limit) ||
    body.limit < 1 ||
    !Array.isArray(body.items) ||
    body.items.length > body.limit ||
    body.items.length > body.total
  )
    throw invalidResponse(status)
  return { total: body.total, page: body.page, limit: body.limit, items: body.items.map(parse) }
}
function nullableString(v: unknown): v is string | null {
  return v === null || typeof v === 'string'
}
function channelStatus(v: unknown): v is ChannelStatus {
  return v === 'eligible' || v === 'no_consent' || v === 'no_address'
}
function validDate(v: unknown): v is string {
  return typeof v === 'string' && !Number.isNaN(Date.parse(v))
}

export function parseMember(row: unknown): Member {
  if (
    !isRecord(row) ||
    !isCount(row.id) ||
    row.id < 1 ||
    !nullableString(row.first_name) ||
    typeof row.city !== 'string' ||
    !nullableString(row.email) ||
    !nullableString(row.phone) ||
    !channelStatus(row.sms_status) ||
    !channelStatus(row.email_status)
  )
    throw invalidResponse(200)
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

export function parseTemplate(row: unknown): ContentTemplate {
  if (
    !isRecord(row) ||
    typeof row.id !== 'string' ||
    typeof row.name !== 'string' ||
    typeof row.sms_text !== 'string' ||
    typeof row.email_subject !== 'string' ||
    typeof row.email_body !== 'string' ||
    !validDate(row.created_at) ||
    !validDate(row.updated_at)
  )
    throw invalidResponse(200)
  return {
    id: row.id,
    name: row.name,
    sms_text: row.sms_text,
    email_subject: row.email_subject,
    email_body: row.email_body,
    created_at: row.created_at,
    updated_at: row.updated_at,
  }
}

export async function getOverview(signal?: AbortSignal): Promise<Overview> {
  const { status, body } = await request('/api/overview', signal)
  if (
    !isRecord(body) ||
    !isCount(body.campaigns) ||
    !isCount(body.draft) ||
    !isCount(body.approved) ||
    !isCount(body.sending) ||
    !isCount(body.completed) ||
    !isCount(body.users) ||
    !isCount(body.sent) ||
    !isCount(body.failed) ||
    !isCount(body.pending) ||
    !isCount(body.recovered) ||
    !isCount(body.templates) ||
    body.draft + body.approved + body.sending + body.completed !== body.campaigns ||
    body.recovered > body.sent
  )
    throw invalidResponse(status)
  return {
    campaigns: body.campaigns,
    draft: body.draft,
    approved: body.approved,
    sending: body.sending,
    completed: body.completed,
    users: body.users,
    sent: body.sent,
    failed: body.failed,
    pending: body.pending,
    recovered: body.recovered,
    templates: body.templates,
  }
}

export async function getCampaignPage(
  page: number,
  state: string,
  signal?: AbortSignal,
): Promise<Page<CampaignSummary>> {
  const { status, body } = await request(
    `/api/campaigns?page=${page}&limit=10&status=${encodeURIComponent(state)}`,
    signal,
  )
  const campaigns = parseCampaignList(status, body)
  if (!isRecord(body)) throw invalidResponse(status)
  return parsePage(status, { ...body, items: campaigns }, (row) => {
    const [item] = parseCampaignList(status, { campaigns: [row] })
    if (!item) throw invalidResponse(status)
    return item
  })
}

export async function getMembers(
  page: number,
  search: string,
  campaignId?: string,
  signal?: AbortSignal,
): Promise<Page<Member>> {
  const path = campaignId
    ? `/api/campaigns/${encodeURIComponent(campaignId)}/audience`
    : '/api/audience'
  const { status, body } = await request(
    `${path}?page=${page}&limit=10&search=${encodeURIComponent(search)}`,
    signal,
  )
  return parsePage(status, body, parseMember)
}

export async function getTemplates(page = 1, signal?: AbortSignal): Promise<Page<ContentTemplate>> {
  const { status, body } = await request(`/api/templates?page=${page}&limit=10`, signal)
  return parsePage(status, body, parseTemplate)
}

export async function saveTemplate(
  id: string | null,
  name: string,
  fields: ContentFields,
  signal?: AbortSignal,
): Promise<ContentTemplate> {
  const path = id ? `/api/templates/${encodeURIComponent(id)}` : '/api/templates'
  const { body } = await request(path, signal, {
    method: id ? 'PUT' : 'POST',
    body: JSON.stringify({ name, ...fields }),
  })
  return parseTemplate(body)
}

export async function deleteTemplate(id: string, signal?: AbortSignal): Promise<void> {
  await request(`/api/templates/${encodeURIComponent(id)}`, signal, { method: 'DELETE' })
}
