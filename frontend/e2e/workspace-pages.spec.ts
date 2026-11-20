import { expect, test } from '@playwright/test'
import { createCampaign, saveButton } from './helpers.ts'

test('çalışma alanı menüsü, genel bakış, kampanya filtresi ve kişi araması gerçek API ile çalışır', async ({
  page,
}) => {
  await createCampaign(page)
  await page.goto('/overview')
  await expect(page.getByRole('heading', { level: 1, name: 'Genel bakış' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Kitledeki kişiler' })).toBeVisible()
  const menu = page.getByRole('navigation', { name: 'Sayfalar' })
  await expect(menu.getByRole('link')).toHaveCount(5)
  await expect(page.getByTestId('brand-link')).toHaveText('Insider Campaign Tool')
  await expect(page.getByTestId('brand-link').locator('svg')).toHaveCount(0)
  await menu.getByRole('link', { name: 'Kampanyalar', exact: true }).click()
  await expect(page.getByTestId('campaign-page-row').first()).toBeVisible()
  await page.getByLabel('Durum', { exact: true }).selectOption('draft')
  await expect(page.getByTestId('campaign-page-row').first()).toContainText('Taslak')
  await menu.getByRole('link', { name: 'Kitleler', exact: true }).click()
  await expect(page.getByTestId('member-row')).toHaveCount(10)
  await page.getByLabel('Kişi ara').fill('nonexistent-person-unique')
  await page.getByRole('button', { name: 'Ara', exact: true }).click()
  await expect(page.getByText('Bu sayfada kişi bulunamadı.')).toBeVisible()
  await page.getByLabel('Kişi ara').fill('')
  await page.getByRole('button', { name: 'Ara', exact: true }).click()
  await expect(page.getByTestId('member-row')).toHaveCount(10)
})

test('şablon oluşturulur, düzenlenir, kampanyaya kaydedilir; şablonu silmek kampanyayı değiştirmez', async ({
  page,
}) => {
  const name = `Sepet ${Date.now()}`
  await page.goto('/templates')
  await page.getByLabel('Şablon adı').fill(name)
  await page.getByLabel('SMS metni').fill('Merhaba {{first_name}}, sepetin seni bekliyor!')
  await page.getByLabel('E-posta konusu').fill('Sepetine özel fırsat')
  await page
    .getByLabel('E-posta gövdesi')
    .fill('Merhaba {{first_name}}, alışverişini tamamlayabilirsin.')
  await page.getByRole('button', { name: 'Şablonu kaydet', exact: true }).click()
  await expect(
    page.getByText('Şablon kaydedildi. Kampanyanın içerik ekranında kullanabilirsiniz.'),
  ).toBeVisible()
  await page.reload()
  await page.getByRole('button', { name: `Düzenle ${name}` }).click()
  await page.getByLabel('E-posta konusu').fill('Yeni fırsat')
  await page.getByRole('button', { name: 'Şablonu kaydet', exact: true }).click()
  await expect(
    page.getByText('Şablon kaydedildi. Kampanyanın içerik ekranında kullanabilirsiniz.'),
  ).toBeVisible()
  const id = await createCampaign(page)
  await page.getByRole('button', { name: 'Şablondan doldur', exact: true }).click()
  await page.getByRole('button', { name, exact: true }).click()
  await expect(page.getByLabel('E-posta konusu')).toHaveValue('Yeni fırsat')
  await expect(page.getByTestId('version')).toHaveCount(0)
  await saveButton(page).click()
  await expect(page.getByTestId('version')).toContainText('Sürüm 1')
  await page.goto('/templates')
  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: `Sil ${name}`, exact: true }).click()
  await expect(page.getByText('Şablon silindi.')).toBeVisible()
  await page.goto(`/campaigns/${id}`)
  await expect(page.getByTestId('version')).toContainText('Sürüm 1')
  await expect(page.getByLabel('E-posta konusu')).toHaveValue('Yeni fırsat')
})

test('geçersiz şablon kaydedilmez ve dar ekranda yeni sayfalar taşmaz', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 800 })
  for (const path of ['/overview', '/campaigns', '/audiences', '/templates']) {
    await page.goto(path)
    await expect(page.locator('main h1')).toBeVisible()
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth))
      .toBeLessThanOrEqual(0)
  }
  await page.getByLabel('Şablon adı').fill('Geçersiz')
  await page.getByLabel('SMS metni').fill('{{password}}')
  await page.getByLabel('E-posta konusu').fill('Konu')
  await page.getByLabel('E-posta gövdesi').fill('Gövde')
  await page.getByRole('button', { name: 'Şablonu kaydet', exact: true }).click()
  await expect(page.getByTestId('error-panel')).toContainText('Şablon içeriği geçersiz')
  await expect(page.getByLabel('SMS metni')).toHaveAttribute('aria-invalid', 'true')
  // Temizlenmeden ayrılmak kullanıcı onayı ister.
  page.once('dialog', (dialog) => dialog.dismiss())
  await page.getByTestId('brand-link').click()
  await expect(page).toHaveURL(/\/templates$/)
})
