import { expect, test } from '@playwright/test'
import {
  approve,
  BRIEF,
  EXPECTED,
  expectAudience,
  generateContent,
  saveButton,
  serverState,
  smsField,
} from './helpers.ts'

test('brief → kitle → içerik → düzenle → kaydet → onayla → yenileyince onaylı durum korunur', async ({
  page,
}) => {
  // 1) Brief gönder, kitle sayılarını ve anlaşılan koşulları gör.
  await page.goto('/')
  await page.getByLabel('Brief').fill(BRIEF)
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expectAudience(page)
  await expect(page.getByText('Şehir: İstanbul')).toBeVisible()
  await expect(page.getByText('Son 30 günde sepete ürün eklemiş')).toBeVisible()
  await expect(page.getByText('Son 30 günde satın alma yapmamış')).toBeVisible()

  await page.getByRole('link', { name: 'İçeriği hazırla' }).click()
  await page.waitForURL(/\/campaigns\/[0-9a-f-]{36}$/)
  const id = page.url().split('/').pop()!
  await expectAudience(page)

  // 2) İçeriği üret (SSE): doğrulanmış sürüm 1.
  await generateContent(page)
  await expect(smsField(page)).toHaveValue(/\{\{first_name\}\}.*%30/)

  // 3) Düzenle ve yeni sürüm olarak kaydet: sürüm 2.
  const edited = 'Merhaba {{first_name}}, sepetindeki ürünlerde %30 indirim seni bekliyor!'
  await smsField(page).fill(edited)
  await expect(page.getByText('Kaydedilmemiş değişiklik var.')).toBeVisible()
  await saveButton(page).click()
  await expect(page.getByTestId('version')).toHaveText('Sürüm 2 · Elle düzenlendi')
  await expect(page.getByText('Kaydedilmemiş değişiklik var.')).toBeHidden()

  // 4) Onayla: kuyruğa alınan sayılar önizlemedeki kitleyle aynıdır (kontrollü veri).
  await approve(page, 2)
  await expect(page.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 2')
  await expect(page.getByTestId('queued')).toContainText(
    `${EXPECTED.sms} SMS ve ${EXPECTED.email} e-posta mesajı`,
  )
  await expect(smsField(page)).toHaveAttribute('readonly', '')
  await expect(page.getByRole('button')).toHaveCount(0) // işlem düğmesi kalmadı

  // 5) Sayfayı yenile: onaylı durum ve içerik sunucudan geri yüklenir.
  await page.reload()
  await expect(page.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 2')
  await expect(page.getByTestId('version')).toHaveText('Sürüm 2 · Elle düzenlendi')
  await expect(smsField(page)).toHaveValue(edited)
  await expect(smsField(page)).toHaveAttribute('readonly', '')
  await expect(page.getByRole('button')).toHaveCount(0)

  // Arayüzden bağımsız doğrulama: sunucuda kampanya onaylı ve sürüm 2.
  const state = await serverState(page, id)
  expect(state.status).toBe('approved')
  expect(state.approved_content_version).toBe(2)
  expect(state.content?.sms_text).toBe(edited)
})
