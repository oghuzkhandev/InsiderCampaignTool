import { expect, test } from '@playwright/test'
import { approvedCampaign, EXPECTED, serverState } from './helpers.ts'

// Gerçek yığın: HİÇBİR ŞEY taklit edilmez (page.route yok).
//
// Kullanıcı gibi kampanya oluşturulur, içerik üretilir ve onaylanır; ardından
// dashboard açılır. Gönderimi gerçek worker yapar, gerçek mock provider'a gider ve
// dashboard sayıları gerçek backend SSE akışından (GET /api/campaigns/{id}/events)
// gelir. Bu test yalnızca `delivery` profiliyle koşar: `make e2e-delivery`
// (compose.e2e.yaml; worker ve mock-provider yalnızca o profilde açılır).
//
// Provider `accept_then_drop` modundadır (compose.e2e.yaml): her mesajın ilk kabul
// yanıtı düşer, worker aynı Idempotency-Key ile yeniden dener ve provider kayıtlı
// kabulü döndürür. Sonuç deterministiktir ve yanıt kaybı yüzünden mükerrer gönderim
// olmaz: 7 mesajın 7'si gönderildi, 0 başarısız.

const MESSAGES = EXPECTED.sms + EXPECTED.email

test(
  'onaylanan kampanya gerçek worker, provider ve SSE ile gönderilir; dashboard tamamlandığını gösterir',
  { tag: '@real' },
  async ({ page }) => {
    test.setTimeout(90_000)
    const id = await approvedCampaign(page)

    // Akış gerçek backend'den gelmeli: yanıtı bekleyip türünü doğrula.
    const stream = page.waitForResponse(
      (response) => response.url().endsWith(`/api/campaigns/${id}/events`) && response.ok(),
    )
    await page.getByRole('link', { name: 'Gönderimi izle' }).click()
    await page.waitForURL(new RegExp(`/campaigns/${id}/delivery$`))
    await expect(page.getByRole('heading', { level: 1, name: 'Gönderim takibi' })).toBeVisible()
    expect((await stream).headers()['content-type']).toContain('text/event-stream')

    // Worker bitirene kadar bekle: durum Tamamlandı olur ve takip kapanır.
    await expect(page.getByTestId('campaign-status')).toHaveText('Tamamlandı', { timeout: 60_000 })

    await expect(page.getByTestId('total-messages')).toHaveText(String(MESSAGES))
    await expect(page.getByTestId('count-sent')).toHaveText(String(MESSAGES))
    for (const key of ['queued', 'retrying', 'sending', 'failed', 'skipped']) {
      await expect(page.getByTestId(`count-${key}`)).toHaveText('0')
    }
    await expect(
      page.getByRole('progressbar', { name: 'İşi biten mesajların oranı' }),
    ).toHaveAttribute('aria-valuenow', '100')
    await expect(page.getByTestId('note-exhausted')).toHaveCount(0)
    await expect(page.getByTestId('note-rejected')).toHaveCount(0)
    await expect(page.getByTestId('completed-note')).toBeVisible()
    await expect(page.getByTestId('connection-status')).toContainText('Canlı takip kapatıldı')

    // Arayüzden bağımsız doğrulama: sunucudaki kampanya da tamamlandı.
    expect((await serverState(page, id)).status).toBe('completed')

    // Yenilenince son durum yeniden kurulur: akış ilk görüntüde tamamlandığını bildirir.
    await page.reload()
    await expect(page.getByTestId('campaign-status')).toHaveText('Tamamlandı')
    await expect(page.getByTestId('count-sent')).toHaveText(String(MESSAGES))
  },
)
