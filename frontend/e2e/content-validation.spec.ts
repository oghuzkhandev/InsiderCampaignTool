import { expect, test } from '@playwright/test'
import {
  approveButton,
  createCampaign,
  generateContent,
  saveButton,
  serverState,
  smsField,
} from './helpers.ts'

// Her test kendi kampanyasını oluşturur; birbirine bağımlı değildir.

test('desteklenmeyen değişken: kaydetme reddedilir ve onay kapalı kalır', async ({ page }) => {
  const id = await createCampaign(page)
  await generateContent(page)
  const original = await smsField(page).inputValue()

  await smsField(page).fill('Merhaba {{city}}, %30 indirim!')
  await saveButton(page).click()

  // Hata alanın altında Türkçe gösterilir; yeni sürüm oluşmaz.
  await expect(page.locator('#sms-problems')).toContainText('bilinmeyen değişken "{{city}}"')
  await expect(page.locator('#sms-problems')).toContainText('{{first_name}}')
  await expect(page.getByTestId('version')).toHaveText('Sürüm 1 · Yapay zekâ ile üretildi')
  // Onay kapalı: kayıtlı sürüm 1 ile ekrandaki (reddedilen) metin farklı.
  await expect(approveButton(page, 1)).toBeDisabled()
  await expect(page.getByTestId('approved')).toHaveCount(0)

  // Sunucuda hiçbir şey değişmedi: hâlâ sürüm 1 ve taslak.
  const state = await serverState(page, id)
  expect(state.status).toBe('draft')
  expect(state.content?.content_version).toBe(1)
  expect(state.content?.sms_text).toBe(original)
})

test('kaydedilmemiş düzenleme: kaydetmeden onaylanamaz', async ({ page }) => {
  const id = await createCampaign(page)
  await generateContent(page)
  await expect(approveButton(page, 1)).toBeEnabled() // değişiklik yokken onaya hazır

  await smsField(page).fill('Merhaba {{first_name}}, bu metin henüz kaydedilmedi.')

  await expect(page.getByText('Kaydedilmemiş değişiklik var.')).toBeVisible()
  await expect(approveButton(page, 1)).toBeDisabled()
  await expect(page.getByText('önce değişiklikleri kaydedin')).toBeVisible()
  await expect(page.getByTestId('approved')).toHaveCount(0)

  // Sunucuda onay yok; sayfa yenilenince kaydedilmemiş metin gider, onay yeniden açılır.
  const state = await serverState(page, id)
  expect(state.status).toBe('draft')
  expect(state.approved_content_version).toBeUndefined()
  // Kaydedilmemiş metin varken yenilemek tarayıcının çıkış uyarısını çıkarır (içerik kaybolmasın).
  let dialogType = ''
  page.once('dialog', (dialog) => {
    dialogType = dialog.type()
    void dialog.accept() // kullanıcı "yine de yenile" der
  })
  await page.reload()
  expect(dialogType).toBe('beforeunload')
  await expect(page.getByTestId('version')).toHaveText('Sürüm 1 · Yapay zekâ ile üretildi')
  await expect(smsField(page)).not.toHaveValue(/henüz kaydedilmedi/)
  await expect(approveButton(page, 1)).toBeEnabled()
})
