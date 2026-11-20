import { expect, test, type Page } from '@playwright/test'
import { approvedCampaign, BRIEF, createCampaign, expectAudience } from './helpers.ts'

// CMP-001: ana sayfada "Son kampanyalar". Her test kendi kampanyasını ayırt edici bir brief ile
// oluşturur (brief'in sonuna eklenen ek kitle koşullarını değiştirmez), böylece ortamda başka
// kampanyalar olsa da kendi satırını bulur. Testler tek worker'la sırayla koşar (en yeni başta).

let counter = 0
const uniqueBrief = (label: string) => `${BRIEF} (${label} ${Date.now()}-${++counter})`

const recent = (page: Page) => page.getByTestId('recent-campaigns')
const rows = (page: Page) => recent(page).getByTestId('recent-row')

test('yeni kampanya listenin başında: brief, durum ve tarih görünür; taslak kampanya ekranına açılır', async ({
  page,
}) => {
  const brief = uniqueBrief('taslak')
  const id = await createCampaign(page, brief)

  await page.goto('/')
  const first = rows(page).first()
  await expect(first.getByTestId('recent-brief')).toHaveText(brief)
  await expect(first.getByTestId('recent-status')).toHaveText('Taslak')
  await expect(first.locator('time')).toHaveAttribute('datetime', /^\d{4}-\d{2}-\d{2}T/)
  await expect(first.locator('time')).not.toBeEmpty()
  await expect(first.getByTestId('recent-target')).toContainText('Kampanyayı aç')
  await expect(first).toHaveAttribute('href', `/campaigns/${id}`)
  // Kitle sayısı satırda gösterilmez (şimdilik).
  await expect(first).not.toContainText('SMS')

  await first.click()
  await page.waitForURL(new RegExp(`/campaigns/${id}$`))
  await expect(page.getByRole('heading', { level: 1, name: 'Kampanya' })).toBeVisible()
  await expect(page.getByTestId('campaign-state')).toHaveText('Taslak')
})

test('onaylanmış kampanya gönderim takibine açılır', async ({ page }) => {
  const brief = uniqueBrief('onaylı')
  const id = await approvedCampaign(page, brief)

  await page.goto('/')
  const first = rows(page).first()
  await expect(first.getByTestId('recent-brief')).toHaveText(brief)
  await expect(first.getByTestId('recent-status')).toHaveText('Onaylandı, kuyrukta')
  await expect(first.getByTestId('recent-target')).toContainText('Gönderimi izle')
  await expect(first).toHaveAttribute('href', `/campaigns/${id}/delivery`)

  await first.click()
  await page.waitForURL(new RegExp(`/campaigns/${id}/delivery$`))
  await expect(page.getByRole('heading', { level: 1, name: 'Gönderim takibi' })).toBeVisible()
})

test('en yeni kampanya başta: iki kampanya oluşturma sırasının tersiyle listelenir', async ({
  page,
}) => {
  const older = uniqueBrief('eski')
  const newer = uniqueBrief('yeni')
  await createCampaign(page, older)
  await createCampaign(page, newer)

  await page.goto('/')

  await expect(rows(page).nth(0).getByTestId('recent-brief')).toHaveText(newer)
  await expect(rows(page).nth(1).getByTestId('recent-brief')).toHaveText(older)
})

test('aynı sayfada yeni kampanya oluşunca liste yenilenir ve yeni kampanya en üstte görünür', async ({
  page,
}) => {
  await page.goto('/')
  await expect(recent(page)).toBeVisible()
  const brief = uniqueBrief('canlı')

  await page.getByLabel('Brief').fill(brief)
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expectAudience(page)

  await expect(rows(page).first().getByTestId('recent-brief')).toHaveText(brief)
})

test('liste satırı sayfa yenilenince de aynı yerdedir (sunucudan gelir)', async ({ page }) => {
  const brief = uniqueBrief('kalıcı')
  await createCampaign(page, brief)

  await page.goto('/')
  await page.reload()

  await expect(rows(page).first().getByTestId('recent-brief')).toHaveText(brief)
})

test('API: varsayılan 10, en fazla 20; geçersiz sınır 400; yalnızca belgelenen alanlar', async ({
  page,
}) => {
  // 21 kampanya oluştur: hem varsayılanı (10) hem üst sınırı (20) kesin sınamak için.
  for (let i = 0; i < 21; i++) {
    const created = await page.request.post('/api/campaigns', {
      data: { brief: uniqueBrief(`api-${i}`) },
    })
    expect(created.status()).toBe(201)
  }

  const ids = async (query: string) => {
    const res = await page.request.get(`/api/campaigns${query}`)
    expect(res.status()).toBe(200)
    const body = (await res.json()) as { campaigns: Record<string, unknown>[] }
    return body.campaigns
  }

  expect(await ids('')).toHaveLength(10)
  expect(await ids('?limit=3')).toHaveLength(3)
  expect(await ids('?limit=20')).toHaveLength(20)
  expect(await ids('?limit=25')).toHaveLength(20) // 20'ye indirilir
  expect(await ids('?limit=1000')).toHaveLength(20)

  const list = await ids('?limit=20')
  for (const row of list) {
    expect(Object.keys(row).sort()).toEqual(['brief', 'created_at', 'id', 'status'])
  }
  // En yeni başta: created_at azalan.
  const times = list.map((r) => Date.parse(String(r.created_at)))
  expect(times).toEqual([...times].sort((a, b) => b - a))

  for (const bad of ['0', '-1', 'abc', '2.5']) {
    const res = await page.request.get(`/api/campaigns?limit=${bad}`)
    expect(res.status(), `limit=${bad}`).toBe(400)
    expect(((await res.json()) as { error: { code: string } }).error.code).toBe('INVALID_REQUEST')
  }
})
