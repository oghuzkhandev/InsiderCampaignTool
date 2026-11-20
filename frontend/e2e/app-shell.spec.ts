import { expect, test, type Page } from '@playwright/test'
import {
  approve,
  approvedCampaign,
  BRIEF,
  createCampaign,
  generateContent,
  smsField,
} from './helpers.ts'

// Uygulama kabuğu (masaüstü): üst başlıktaki "Campaign Tool" ana sayfaya döner, solda kenar
// çubuğu "Yeni kampanya" ve "Son kampanyalar" taşır. Telefondaki çekmece davranışı
// `mobile-layout.spec.ts` içinde sınanır.

const sidebar = (page: Page) => page.locator('div[data-slot="sidebar"]')

test('"Campaign Tool" başlığı her ekrandan ana sayfaya döner; sayfa başlığı (h1) tek kalır', async ({
  page,
}) => {
  const id = await approvedCampaign(page)

  for (const [path, heading] of [
    [`/campaigns/${id}`, 'Kampanya'],
    [`/campaigns/${id}/delivery`, 'Gönderim takibi'],
  ] as const) {
    await page.goto(path)
    await expect(page.getByRole('heading', { level: 1, name: heading })).toBeVisible()
    await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)

    const brand = page.getByTestId('brand-link')
    await expect(brand).toHaveText('Campaign Tool')
    await expect(brand).toHaveAttribute('href', '/')
    await brand.click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole('heading', { level: 1, name: 'Kampanya önizleme' })).toBeVisible()
  }
})

test('kenar çubuğu masaüstünde açıktır; düğmeyle kapanıp açılır, "Yeni kampanya" brief formuna götürür', async ({
  page,
}) => {
  const id = await createCampaign(page)

  await expect(sidebar(page)).toHaveAttribute('data-state', 'expanded')
  await expect(page.getByTestId('recent-campaigns')).toBeVisible()
  // Açık kampanyanın satırı işaretlidir (aria-current).
  await expect(page.getByTestId('recent-row').first()).toHaveAttribute('href', `/campaigns/${id}`)
  await expect(page.getByTestId('recent-row').first()).toHaveAttribute('aria-current', 'page')

  const toggle = page.getByRole('button', { name: 'Kenar çubuğunu aç veya kapat' })
  await toggle.click()
  await expect(sidebar(page)).toHaveAttribute('data-state', 'collapsed')
  await toggle.click()
  await expect(sidebar(page)).toHaveAttribute('data-state', 'expanded')

  await page.getByTestId('sidebar-new-campaign').click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByLabel('Brief')).toBeVisible()
})

test('kenar çubuğundaki liste kampanya durumunu onayla birlikte günceller (sayfa yenilemeden)', async ({
  page,
}) => {
  await createCampaign(page)
  const row = page.getByTestId('recent-row').first()
  await expect(row.getByTestId('recent-status')).toHaveText('Taslak')

  await generateContent(page)
  await approve(page, 1)
  await expect(page.getByTestId('approved')).toBeVisible()

  await expect(row.getByTestId('recent-status')).toHaveText('Onaylandı, kuyrukta')
  await expect(row).toHaveAttribute('href', /\/delivery$/)
})

test('kaydedilmemiş değişiklik varken başlık ve kenar çubuğu bağlantıları uyarır; onaylanırsa çıkılır', async ({
  page,
}) => {
  await createCampaign(page)
  await generateContent(page)
  await smsField(page).fill('Merhaba {{first_name}}, bu metin henüz kaydedilmedi.')
  await expect(page.getByText('Kaydedilmemiş değişiklik var.')).toBeVisible()

  for (const link of [page.getByTestId('brand-link'), page.getByTestId('sidebar-new-campaign')]) {
    const dialogs: string[] = []
    page.once('dialog', (dialog) => {
      dialogs.push(dialog.message())
      void dialog.dismiss()
    })
    await link.click()
    await expect.poll(() => dialogs.length).toBe(1)
    expect(dialogs[0]).toContain('Kaydedilmemiş değişiklikleriniz var')
    await expect(page).toHaveURL(/\/campaigns\/[0-9a-f-]{36}$/)
  }

  page.once('dialog', (dialog) => void dialog.accept())
  await page.getByTestId('brand-link').click()
  await expect(page).toHaveURL(/\/$/)
})

test('kenar çubuğunda kampanya brief ve durumu görünür; önizleme ekranında liste kartı yoktur', async ({
  page,
}) => {
  await page.request.post('/api/campaigns', { data: { brief: BRIEF } })
  await page.goto('/')

  await expect(page.getByRole('main').getByTestId('recent-campaigns')).toHaveCount(0)
  await expect(sidebar(page).getByTestId('recent-campaigns')).toBeVisible()
  await expect(
    sidebar(page).getByRole('heading', { level: 2, name: 'Son kampanyalar' }),
  ).toBeVisible()
})
