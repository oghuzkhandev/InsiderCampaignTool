import { expect, test } from '@playwright/test'
import {
  approve,
  BRIEF,
  createCampaign,
  expectAudience,
  generateContent,
  smsField,
} from './helpers.ts'

// Akışta Geri / İleri gezintisi, örnek brief önerileri ve kaydedilmemiş değişiklik uyarısı.
// Her test kendi kampanyasını oluşturur.

test('boş ekranda örnek brief önerilir; seçince alan dolar ve kitle hesaplanabilir', async ({
  page,
}) => {
  await page.goto('/')
  const examples = page.getByTestId('example-brief')
  await expect(examples).toHaveCount(3)

  // İlk örnek, kontrollü veri için bilinen sayıları verir (CASE.md örnek brief'i).
  await examples.first().click()
  await expect(page.getByLabel('Brief')).toHaveValue(BRIEF)
  await expect(page.getByTestId('examples')).toHaveCount(0) // yazılmış metnin üzerine öneri çıkmaz
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expectAudience(page)

  // Diğer örnekler de desteklenir: hata değil, kitle önizlemesi gelir.
  await page.getByLabel('Brief').fill('')
  await page.getByTestId('example-brief').nth(1).click()
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expect(page.getByTestId('total')).toBeVisible()
  await expect(page.getByTestId('error-panel')).toHaveCount(0)
})

test('Geri ve İleri: önizleme → içerik → onay → gönderim takibi → yeni kampanya', async ({
  page,
}) => {
  // 1. Kitle: "Geri" yok, "İleri" kitle hesaplanana kadar kapalı.
  await page.goto('/')
  // `main` ile sınırlı: kenar çubuğundaki kampanya satırlarının brief metni "geri" içerebilir.
  const main = page.getByRole('main')
  await expect(main.getByRole('link', { name: /Geri/ })).toHaveCount(0)
  await expect(page.getByRole('button', { name: /İleri: İçeriği hazırla/ })).toBeDisabled()
  await expect(page.getByText('Önce kitleyi hesaplayın.')).toBeVisible()

  await page.getByLabel('Brief').fill(BRIEF)
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expectAudience(page)

  // 2. İçerik: "İleri" ile geçilir; gönderim takibi onaya kadar kapalı.
  await page.getByRole('link', { name: 'İleri: İçeriği hazırla' }).click()
  await page.waitForURL(/\/campaigns\/[0-9a-f-]{36}$/)
  const id = page.url().split('/').pop()!
  await expect(page.getByRole('button', { name: /İleri: Gönderimi izle/ })).toBeDisabled()
  await expect(page.getByText('Önce içeriği onaylayın.')).toBeVisible()
  // Sol düğme önceki aşamaya dönmez, yeni kampanya başlatır: "Geri" değil kendi adıyla anılır.
  await expect(page.getByRole('link', { name: '+ Yeni kampanya' })).toBeVisible()
  await expect(page.getByRole('main').getByRole('link', { name: /Geri/ })).toHaveCount(0)

  // 3. Son onay başarılı olunca gönderim takibi kendiliğinden açılır.
  await generateContent(page)
  await approve(page, 1)
  await page.waitForURL(new RegExp(`/campaigns/${id}/delivery$`))

  // 4. Gönderim takibi: "Geri" kampanyaya döner. Worker bu ortamda kapalı, gönderim bitmedi;
  // yine de yeni kampanya açılabilir ve gönderimin arka planda sürdüğü yazar.
  await expect(page.getByRole('link', { name: 'İleri: Yeni kampanya' })).toBeVisible()
  await expect(page.getByText('Gönderim arka planda devam eder.')).toBeVisible()
  await page.getByRole('link', { name: 'Geri: Kampanyaya dön' }).click()
  await page.waitForURL(new RegExp(`/campaigns/${id}$`))
  await expect(page.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 1')
})

test('kaydedilmemiş değişiklik varken "Yeni kampanya" uyarır; vazgeçilirse metin korunur, onaylanırsa çıkılır', async ({
  page,
}) => {
  await createCampaign(page)
  await generateContent(page)
  const edited = 'Merhaba {{first_name}}, bu metin henüz kaydedilmedi.'
  await smsField(page).fill(edited)
  await expect(page.getByText('Kaydedilmemiş değişiklik var.')).toBeVisible()

  // Vazgeç: sayfada kalınır ve yazılan metin durur.
  const dialogs: string[] = []
  page.once('dialog', (dialog) => {
    dialogs.push(dialog.message())
    void dialog.dismiss()
  })
  await page.getByRole('link', { name: '+ Yeni kampanya' }).click()
  await expect.poll(() => dialogs.length).toBe(1)
  expect(dialogs[0]).toContain('Kaydedilmemiş değişiklikleriniz var')
  await expect(page).toHaveURL(/\/campaigns\/[0-9a-f-]{36}$/)
  await expect(smsField(page)).toHaveValue(edited)

  // Onayla: çıkılır.
  page.once('dialog', (dialog) => void dialog.accept())
  await page.getByRole('link', { name: '+ Yeni kampanya' }).click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByRole('heading', { level: 1, name: 'Kampanya önizleme' })).toBeVisible()
})

test('kaydedilmiş içerikle ayrılırken uyarı çıkmaz', async ({ page }) => {
  await createCampaign(page)
  await generateContent(page)

  let dialogCount = 0
  page.on('dialog', (dialog) => {
    dialogCount++
    void dialog.dismiss()
  })
  await page.getByRole('link', { name: '+ Yeni kampanya' }).click()
  await expect(page).toHaveURL(/\/$/)
  expect(dialogCount).toBe(0)
})
