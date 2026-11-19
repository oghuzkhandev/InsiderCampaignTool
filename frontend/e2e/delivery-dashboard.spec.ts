import { expect, test, type Page, type Route } from '@playwright/test'
import { approvedCampaign, createCampaign, EXPECTED } from './helpers.ts'

// Gönderim takibi ekranı (TRK-001): hata, yeniden bağlanma, mobil ve erişilebilirlik
// senaryoları.
//
// Kampanya oluşturma, içerik üretimi, onay ve kampanya yükleme GERÇEK backend ile
// çalışır. Yalnızca dashboard'un canlı olay akışı (GET /api/campaigns/{id}/events)
// Playwright ile taklit edilir: gerçek bir akışı bilerek koparmak, 403/500 döndürmek
// veya sessiz bırakmak (takılma) güvenilir biçimde ancak böyle tetiklenir. Taklit
// sayıların toplamı, gerçek onayın kuyruğa aldığı mesaj sayısıdır (SMS + e-posta).
//
// Gerçek worker, provider ve SSE ile gönderimin tamamlandığını gösteren test
// ayrıdır: delivery-real.spec.ts (@real, `make e2e-delivery`).

/** Onayın kuyruğa aldığı toplam mesaj sayısı (kontrollü veri kümesi: 3 SMS + 4 e-posta). */
const MESSAGES = EXPECTED.sms + EXPECTED.email

interface Messages {
  queued: number
  retrying: number
  sending: number
  sent: number
  failed: number
  skipped: number
}

/** Toplamı MESSAGES olan, gönderimin sürdüğü bir an. */
const IN_PROGRESS: Messages = { queued: 3, retrying: 1, sending: 1, sent: 2, failed: 0, skipped: 0 }
/** Toplamı MESSAGES olan, tamamlanmış gönderim: bir mesaj deneme sınırında başarısız oldu. */
const COMPLETED: Messages = { queued: 0, retrying: 0, sending: 0, sent: 5, failed: 1, skipped: 1 }

function snapshotEvent(
  status: string,
  messages: Messages,
  failures = { exhausted: 0, rejected: 0 },
): string {
  return `event: snapshot\ndata: ${JSON.stringify({ status, messages, failures })}\n\n`
}

function fulfillEvents(route: Route, body: string) {
  return route.fulfill({
    status: 200,
    contentType: 'text/event-stream',
    headers: { 'Cache-Control': 'no-store' },
    body,
  })
}

const count = (page: Page, key: string) => page.getByTestId(`count-${key}`)

test('onaylanan kampanyanın gönderimi canlı izlenir; bağlantı koparsa yeniden bağlanır, tamamlanınca takip kapanır', async ({
  page,
}) => {
  const id = await approvedCampaign(page)

  let streamRequests = 0
  await page.route(`**/api/campaigns/${id}/events`, (route) => {
    streamRequests++
    // 1. bağlantı: gönderim sürüyor ve akış kapanıyor (kopma). Sonrakiler: tamamlandı.
    return fulfillEvents(
      route,
      streamRequests === 1
        ? snapshotEvent('sending', IN_PROGRESS)
        : snapshotEvent('completed', COMPLETED, { exhausted: 1, rejected: 0 }),
    )
  })

  // Onay ekranından gönderim takibine geç.
  await page.getByRole('link', { name: 'Gönderimi izle' }).click()
  await page.waitForURL(new RegExp(`/campaigns/${id}/delivery$`))
  await expect(page.getByRole('heading', { level: 1, name: 'Gönderim takibi' })).toBeVisible()

  // Kampanya gerçek backend'den yüklendi (onaylı), sayılar akıştan geliyor.
  await expect(page.getByTestId('total-messages')).toHaveText(String(MESSAGES))
  await expect(count(page, 'queued')).toHaveText('3')
  await expect(count(page, 'retrying')).toHaveText('1')
  await expect(count(page, 'sending')).toHaveText('1')
  await expect(count(page, 'sent')).toHaveText('2')
  await expect(page.getByTestId('campaign-status')).toHaveText('Gönderiliyor')

  // Akış koptu: ekran yeniden bağlanır ve tamamlanmış durumu gösterir (sayılar yenilenir).
  await expect(page.getByTestId('campaign-status')).toHaveText('Tamamlandı')
  await expect(count(page, 'queued')).toHaveText('0')
  await expect(count(page, 'sent')).toHaveText('5')
  await expect(count(page, 'failed')).toHaveText('1')
  await expect(count(page, 'skipped')).toHaveText('1')
  await expect(page.getByTestId('total-messages')).toHaveText(String(MESSAGES))
  await expect(
    page.getByRole('progressbar', { name: 'İşi biten mesajların oranı' }),
  ).toHaveAttribute('aria-valuenow', '100')

  // Başarısızlığın nedeni açıklanır ve kesin teslim edilmedi denmez.
  await expect(page.getByTestId('note-exhausted')).toContainText(
    '1 mesaj: Gönderim sonucu doğrulanamadı; deneme sınırı doldu',
  )
  await expect(page.getByTestId('note-exhausted')).toContainText(
    'kesinlikle teslim edilmediği anlamına gelmez',
  )
  await expect(page.getByTestId('completed-note')).toBeVisible()
  await expect(page.getByTestId('connection-status')).toContainText('Canlı takip kapatıldı')
  expect(streamRequests).toBe(2) // tamamlanınca bir daha bağlanmadı

  // Sayfa yenilenince kampanya ve son durum sunucudan yeniden kurulur.
  await page.reload()
  await expect(page.getByTestId('campaign-status')).toHaveText('Tamamlandı')
  await expect(count(page, 'sent')).toHaveText('5')
  expect(streamRequests).toBe(3)

  // Kampanya ekranına dönülebilir; onaylı durum korunur.
  await page.getByRole('link', { name: 'Kampanyaya dön' }).click()
  await page.waitForURL(new RegExp(`/campaigns/${id}$`))
  await expect(page.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 1')
})

test('canlı takip başlatılamazsa hata gösterilir ve "Yeniden bağlan" ile düzelir', async ({
  page,
}) => {
  const id = await approvedCampaign(page)

  // Yeniden denenmeyen bir hata: ekran vazgeçip hatayı ve düğmeyi gösterir.
  let healthy = false
  await page.route(`**/api/campaigns/${id}/events`, (route) => {
    if (healthy) return fulfillEvents(route, snapshotEvent('sending', IN_PROGRESS))
    return route.fulfill({
      status: 403,
      contentType: 'application/json',
      body: JSON.stringify({
        error: { code: 'FORBIDDEN', message: 'Bu kampanyanın takibine erişilemiyor.' },
      }),
    })
  })

  await page.goto(`/campaigns/${id}/delivery`)
  const failed = page.getByTestId('connection-failed')
  await expect(failed).toBeVisible()
  await expect(failed).toContainText('Bu kampanyanın takibine erişilemiyor.')
  await expect(page.getByTestId('total-messages')).toHaveCount(0) // hiç sayı uydurulmaz

  healthy = true
  await page.getByRole('button', { name: 'Yeniden bağlan' }).click()
  await expect(failed).toBeHidden()
  await expect(page.getByTestId('total-messages')).toHaveText(String(MESSAGES))
  await expect(count(page, 'sending')).toHaveText('1')
})

test('onaylanmamış kampanyada gönderim olmadığı söylenir ve canlı takibe bağlanılmaz (gerçek backend)', async ({
  page,
}) => {
  const id = await createCampaign(page) // yalnızca taslak: içerik üretilmedi, onaylanmadı
  const streamRequests: string[] = []
  page.on('request', (request) => {
    if (request.url().endsWith('/events')) streamRequests.push(request.url())
  })

  await page.goto(`/campaigns/${id}/delivery`)

  await expect(page.getByTestId('not-approved')).toContainText('Bu kampanya henüz onaylanmadı.')
  await expect(page.getByTestId('campaign-status')).toHaveText('Taslak')
  await expect(page.getByTestId('total-messages')).toHaveCount(0)
  await expect(page.getByTestId('connection-status')).toHaveCount(0)
  expect(streamRequests).toHaveLength(0)
})

test('olmayan kampanyada "bulunamadı" gösterilir (gerçek backend)', async ({ page }) => {
  await page.goto('/campaigns/00000000-0000-4000-8000-000000000000/delivery')

  const alert = page.getByTestId('load-error')
  await expect(alert).toContainText('Kampanya bulunamadı.')
  await expect(alert.getByRole('button')).toHaveCount(0) // yeniden denemek anlamsız
  await expect(page.getByTestId('total-messages')).toHaveCount(0)
})

test('dar ekranda (telefon) yatay kaydırma olmaz ve sayılar okunur', async ({ page }) => {
  const id = await approvedCampaign(page)
  await page.route(`**/api/campaigns/${id}/events`, (route) =>
    fulfillEvents(route, snapshotEvent('completed', COMPLETED, { exhausted: 1, rejected: 0 })),
  )
  await page.setViewportSize({ width: 360, height: 740 })

  await page.goto(`/campaigns/${id}/delivery`)
  await expect(count(page, 'sent')).toHaveText('5')

  // Altı sayının hepsi görünür ve sayfa yatay taşmaz.
  for (const key of ['queued', 'retrying', 'sending', 'sent', 'failed', 'skipped']) {
    await expect(count(page, key)).toBeVisible()
  }
  const overflow = await page
    .locator('html')
    .evaluate((root) => root.scrollWidth - root.clientWidth)
  expect(overflow).toBeLessThanOrEqual(0)
})
