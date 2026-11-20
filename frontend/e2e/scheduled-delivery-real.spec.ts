import { expect, test, type Page } from '@playwright/test'
import { approveButton, createCampaign, EXPECTED, generateContent } from './helpers.ts'

// Gerçek tarayıcı → nginx → API → PostgreSQL → worker → mock provider → SSE.
// Saat/fetch/SSE taklit edilmez. datetime-local dakika hassasiyetinde olduğu için
// gerçek bir dakika sınırını bekleriz; test saati hızlandırmak worker/DB'yi sınamazdı.
test.use({ timezoneId: 'Europe/Istanbul', locale: 'tr-TR' })

const MESSAGES = EXPECTED.sms + EXPECTED.email

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function readPlan(body: unknown) {
  if (
    !isRecord(body) ||
    typeof body.status !== 'string' ||
    typeof body.scheduled_at !== 'string' ||
    !Number.isFinite(Date.parse(body.scheduled_at)) ||
    typeof body.approved_at !== 'string'
  ) {
    throw new Error('API yanıtında kayıtlı plan ve onay zamanı bekleniyordu')
  }
  return { status: body.status, scheduledAt: body.scheduled_at, approvedAt: body.approved_at }
}

async function storedPlan(page: Page, id: string) {
  const response = await page.request.get(`/api/campaigns/${id}`)
  expect(response.status()).toBe(200)
  const body: unknown = await response.json()
  return readPlan(body)
}

function futurePlan() {
  // Onay, yenileme ve birkaç gerçek heartbeat için en az 30 sn bırakır.
  // En kötü bekleme 90 sn; sabit sleep yerine gerçek tamamlanma olayı beklenir.
  const at = new Date(Math.ceil((Date.now() + 30_000) / 60_000) * 60_000)
  const parts = new Intl.DateTimeFormat('sv-SE', {
    timeZone: 'Europe/Istanbul',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(at)
  const part = (name: string) => {
    const value = parts.find((item) => item.type === name)?.value
    if (!value) throw new Error(`Tarih parçası eksik: ${name}`)
    return value
  }
  return {
    utc: at.toISOString(),
    milliseconds: at.getTime(),
    local: `${part('year')}-${part('month')}-${part('day')}T${part('hour')}:${part('minute')}`,
  }
}

test(
  'planlanan kampanya erken gönderilmez, yenilemede korunur ve zamanı gelince gerçek worker ile tamamlanır',
  { tag: '@real' },
  async ({ page }) => {
    test.setTimeout(180_000)
    const id = await createCampaign(page)
    await generateContent(page)
    const plan = futurePlan()
    await page.getByRole('radio', { name: 'Tarih ve saat seç', exact: true }).check()
    await expect(page.locator('#send-time-zone')).toContainText('Europe/Istanbul')
    await page.getByLabel('Gönderim tarihi ve saati', { exact: true }).fill(plan.local)
    await approveButton(page, 1).click()
    await expect(page.getByTestId('confirm-schedule')).toContainText('Europe/Istanbul')

    const approvalResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/api/campaigns/${id}/approve`) &&
        response.request().method() === 'POST',
    )
    const streamResponse = page.waitForResponse(
      (result) => result.url().endsWith(`/api/campaigns/${id}/events`) && result.ok(),
    )
    await page.getByTestId('approve-confirm').click()
    const response = await approvalResponse
    expect(response.status()).toBe(200)
    const approvalBody: unknown = await response.json()
    const approved = readPlan(approvalBody)
    expect(Date.parse(approved.scheduledAt)).toBe(plan.milliseconds)
    await expect(page).toHaveURL(new RegExp(`/campaigns/${id}/delivery$`))
    await expect(page.getByTestId('campaign-status')).toHaveText('Planlandı')

    // Tekrarlanan onay aynı planı döndürür; farklı saat planı değiştiremez.
    const replay = await page.request.post(`/api/campaigns/${id}/approve`, {
      data: { content_version: 1, scheduled_at: plan.utc },
    })
    expect(replay.status()).toBe(200)
    const replayBody: unknown = await replay.json()
    expect(readPlan(replayBody)).toEqual(approved)
    const conflict = await page.request.post(`/api/campaigns/${id}/approve`, {
      data: {
        content_version: 1,
        scheduled_at: new Date(plan.milliseconds + 60_000).toISOString(),
      },
    })
    expect(conflict.status()).toBe(409)

    expect((await streamResponse).headers()['content-type']).toContain('text/event-stream')
    await expect(page.getByTestId('campaign-status')).toHaveText('Planlandı')
    await expect(page.getByTestId('schedule-notice')).toContainText('Europe/Istanbul')
    expect(await storedPlan(page, id)).toEqual(approved)

    await page.reload()
    await expect(page.getByTestId('campaign-status')).toHaveText('Planlandı')
    const savedTime = await page
      .getByTestId('schedule-notice')
      .locator('time')
      .getAttribute('datetime')
    expect(savedTime && Date.parse(savedTime)).toBe(plan.milliseconds)

    // Yeni gelen gerçek heartbeat'lerde hâlâ yedi iş sırada: donmuş bir ekranı
    // "erken gönderilmedi" diye saymayız. Deneme/gönderim başlamışsa bu kontroller kırılır.
    const connection = page.getByTestId('connection-status')
    await expect(connection).toContainText('Son güncelleme:')
    for (let update = 0; update < 2; update++) {
      const previous = await connection.innerText()
      await expect(connection).not.toHaveText(previous, { timeout: 8_000 })
      expect(Date.now(), 'erken gönderim kontrolü planlanan saatten önce çalışmalı').toBeLessThan(
        plan.milliseconds,
      )
      await expect(page.getByTestId('count-queued')).toHaveText(String(MESSAGES))
      for (const key of ['retrying', 'sending', 'sent', 'failed', 'skipped']) {
        await expect(page.getByTestId(`count-${key}`)).toHaveText('0')
      }
      expect((await storedPlan(page, id)).status).toBe('approved')
    }

    // Sabit gecikme değil, worker'ın gerçek gönderimi ve gerçek SSE tamamlanması beklenir.
    await expect(page.getByTestId('campaign-status')).toHaveText('Tamamlandı', { timeout: 120_000 })
    expect(Date.now()).toBeGreaterThanOrEqual(plan.milliseconds)
    await expect(page.getByTestId('total-messages')).toHaveText(String(MESSAGES))
    await expect(page.getByTestId('count-sent')).toHaveText(String(MESSAGES))
    await expect(page.getByTestId('count-failed')).toHaveText('0')
    await expect(page.getByTestId('count-queued')).toHaveText('0')
    await expect(page.getByTestId('channel-sms-sent')).toHaveText(String(EXPECTED.sms))
    await expect(page.getByTestId('channel-email-sent')).toHaveText(String(EXPECTED.email))
    await expect(page.getByTestId('recovered-count')).toHaveText(String(MESSAGES))
    await expect(page.getByTestId('connection-status')).toContainText('Canlı takip kapatıldı')

    const lateReplay = await page.request.post(`/api/campaigns/${id}/approve`, {
      data: { content_version: 1, scheduled_at: plan.utc },
    })
    expect(lateReplay.status()).toBe(200)
    const lateBody: unknown = await lateReplay.json()
    expect(readPlan(lateBody)).toEqual({ ...approved, status: 'completed' })
    await page.reload()
    await expect(page.getByTestId('campaign-status')).toHaveText('Tamamlandı')
    await expect(page.getByTestId('count-sent')).toHaveText(String(MESSAGES))
    expect((await storedPlan(page, id)).approvedAt).toBe(approved.approvedAt)
  },
)

test(
  'boş ve geçmiş saat onaylanmaz; API de geçmiş planı reddeder ve kampanya taslak kalır',
  { tag: '@real' },
  async ({ page }) => {
    const id = await createCampaign(page)
    await generateContent(page)
    const submittedApprovals: string[] = []
    page.on('request', (request) => {
      if (request.method() === 'POST' && request.url().endsWith(`/api/campaigns/${id}/approve`))
        submittedApprovals.push(request.url())
    })
    await page.getByRole('radio', { name: 'Tarih ve saat seç', exact: true }).check()
    await approveButton(page, 1).click()
    await expect(page.locator('#send-time-error')).toContainText('Geçerli bir tarih ve saat seçin')
    await page.getByLabel('Gönderim tarihi ve saati', { exact: true }).fill('2020-01-01T12:00')
    await approveButton(page, 1).click()
    await expect(page.locator('#send-time-error')).toContainText('gelecekte')
    await expect(page.getByRole('alertdialog')).toHaveCount(0)
    expect(submittedApprovals).toHaveLength(0)

    const rejected = await page.request.post(`/api/campaigns/${id}/approve`, {
      data: { content_version: 1, scheduled_at: '2020-01-01T09:00:00Z' },
    })
    expect(rejected.status()).toBe(422)
    const error: unknown = await rejected.json()
    expect(error).toMatchObject({ error: { code: 'SCHEDULE_IN_PAST' } })
    await page.reload()
    await expect(page.getByTestId('campaign-state')).toHaveText('Taslak')
    await expect(page.getByTestId('version')).toContainText('Sürüm 1')
    await expect(page.getByTestId('schedule-notice')).toHaveCount(0)
  },
)
