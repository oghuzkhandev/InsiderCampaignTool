import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  getCampaignPage,
  getMembers,
  getOverview,
  getTemplates,
  saveTemplate,
  deleteTemplate,
} from '../workspace'
import { jsonResponse } from '@/__tests__/helpers'

const template = {
  id: 'a',
  name: 'Sepet',
  sms_text: 'Merhaba',
  email_subject: 'Konu',
  email_body: 'Gövde',
  created_at: '2026-11-20T00:00:00Z',
  updated_at: '2026-11-20T00:00:00Z',
}
const overview = {
  campaigns: 3,
  draft: 1,
  approved: 0,
  sending: 1,
  completed: 1,
  users: 10,
  sent: 7,
  failed: 0,
  pending: 2,
  recovered: 1,
  templates: 1,
}
const fields = {
  sms_text: template.sms_text,
  email_subject: template.email_subject,
  email_body: template.email_body,
}
function respond(body: unknown, status = 200) {
  const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => jsonResponse(status, body))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}
afterEach(() => vi.unstubAllGlobals())

describe('çalışma alanı API sınırı', () => {
  it('genel bakış sayılarını doğrular', async () => {
    respond(overview)
    expect(await getOverview()).toEqual(overview)
    respond({ ...overview, campaigns: 99 })
    await expect(getOverview()).rejects.toMatchObject({ code: 'UNEXPECTED_RESPONSE' })
  })
  it('bozuk şablon ve sayfa yanıtını reddeder', async () => {
    for (const body of [
      null,
      { items: [template], total: 0, page: 1, limit: 10 },
      { items: [{ ...template, updated_at: 'bad' }], total: 1, page: 1, limit: 10 },
    ]) {
      respond(body)
      await expect(getTemplates()).rejects.toMatchObject({ code: 'UNEXPECTED_RESPONSE' })
    }
  })
  it('şablon oluşturma ve düzenlemede doğru yöntem ve gövdeyi gönderir', async () => {
    const fetchMock = respond(template)
    expect(await saveTemplate(null, 'Sepet', fields)).toEqual(template)
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/templates',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ name: 'Sepet', ...fields }),
      }),
    )
    await saveTemplate('a', 'Sepet', fields)
    expect(fetchMock).toHaveBeenLastCalledWith(
      '/api/templates/a',
      expect.objectContaining({ method: 'PUT' }),
    )
  })
  it('404 ve doğrulama ayrıntılarını korur', async () => {
    respond({ error: { code: 'TEMPLATE_NOT_FOUND', message: 'Yok', reasons: [] } }, 404)
    await expect(deleteTemplate('absent')).rejects.toMatchObject({
      status: 404,
      code: 'TEMPLATE_NOT_FOUND',
    })
  })
  it('kişi aramasını URL verisi olarak kodlar ve geçersiz uygunluğu reddeder', async () => {
    const fetchMock = respond({ items: [], page: 1, limit: 10, total: 0 })
    await getMembers(1, 'x&limit=100')
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/audience?page=1&limit=10&search=x%26limit%3D100',
      expect.any(Object),
    )
    respond({
      items: [
        {
          id: 1,
          first_name: null,
          city: 'İstanbul',
          phone: null,
          email: null,
          sms_status: 'bad',
          email_status: 'eligible',
        },
      ],
      total: 1,
      page: 1,
      limit: 10,
    })
    await expect(getMembers(1, '')).rejects.toMatchObject({ code: 'UNEXPECTED_RESPONSE' })
  })
  it('kampanya filtresi ve sayfalama metadatasını taşır', async () => {
    const fetchMock = respond({ campaigns: [], total: 0, page: 2, limit: 10 })
    expect(await getCampaignPage(2, 'draft')).toEqual({ items: [], total: 0, page: 2, limit: 10 })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/campaigns?page=2&limit=10&status=draft',
      expect.any(Object),
    )
  })
})
