import { expect, type Page } from '@playwright/test'

/** CASE.md'deki örnek brief (backend seed.E2EBrief ile aynı). */
export const BRIEF =
  "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

/**
 * Bu brief için e2e veri kümesinin kesin kitle sayıları (backend seed.E2EExpected,
 * cmd/seed SEED_DATASET=e2e). Veri tarihleri 30 günlük sınırdan en az 15 gün uzak
 * ve rastgelelik yok; bu yüzden sayılar önizleme ile onay arasında ve ortamın
 * yaşından bağımsız olarak kararlıdır. Biri değişirse ikisi birlikte güncellenir.
 */
export const EXPECTED = { total: 7, sms: 3, email: 4 }

export async function expectAudience(page: Page) {
  await expect(page.getByTestId('total')).toHaveText(String(EXPECTED.total))
  await expect(page.getByTestId('sms')).toHaveText(String(EXPECTED.sms))
  await expect(page.getByTestId('email')).toHaveText(String(EXPECTED.email))
}

/**
 * Bir kullanıcı gibi yeni bir kampanya oluşturur: brief yaz, kitleyi hesapla,
 * içerik ekranına geç. Her test kendi kampanyasını oluşturur; testler birbirinin
 * kampanyasına bağımlı değildir. Kampanya kimliğini döndürür. `brief` verilmezse CASE.md'deki
 * örnek brief kullanılır; verilen brief de aynı koşulları içermelidir (kesin kitle sayıları
 * bunlara bağlıdır), yalnızca sonuna ayırt edici bir ek konabilir.
 */
export async function createCampaign(page: Page, brief: string = BRIEF): Promise<string> {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Kampanya önizleme' })).toBeVisible()
  await page.getByLabel('Brief').fill(brief)
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expectAudience(page)

  await page.getByRole('link', { name: 'İçeriği hazırla' }).click()
  await page.waitForURL(/\/campaigns\/[0-9a-f-]{36}$/)
  await expect(page.getByTestId('total')).toBeVisible() // kampanya yüklendi
  return page.url().split('/').pop()!
}

/** MockLLM ile içerik üretir ve sürüm 1 (yapay zekâ) olarak kaydedilmesini bekler. */
export async function generateContent(page: Page) {
  await page.getByRole('button', { name: 'İçerik üret' }).click()
  await expect(page.getByTestId('version')).toHaveText('Sürüm 1 · Yapay zekâ ile üretildi')
}

/**
 * Bir kullanıcı gibi kampanyayı oluşturur, içerik üretir ve onaylar; onayın
 * kuyruğa aldığı mesaj sayısını doğrular. Kampanya kimliğini döndürür.
 */
export async function approvedCampaign(page: Page, brief: string = BRIEF): Promise<string> {
  const id = await createCampaign(page, brief)
  await generateContent(page)
  await approve(page, 1)
  await expect(page.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 1')
  await expect(page.getByTestId('queued')).toContainText(
    `${EXPECTED.sms} SMS ve ${EXPECTED.email} e-posta mesajı`,
  )
  return id
}

/** Sunucudaki gerçek durum (arayüzden bağımsız, web proxy'si üzerinden okuma). */
export async function serverState(page: Page, id: string) {
  const response = await page.request.get(`/api/campaigns/${id}`)
  expect(response.ok()).toBe(true)
  return (await response.json()) as {
    status: string
    approved_content_version?: number
    content?: { content_version: number; source: string; sms_text: string }
  }
}

export const smsField = (page: Page) => page.getByLabel('SMS metni')
export const approveButton = (page: Page, version: number) =>
  page.getByRole('button', { name: `Onayla (sürüm ${version})` })
export const saveButton = (page: Page) =>
  page.getByRole('button', { name: 'Yeni sürüm olarak kaydet' })

/** İki adımlı onay: düğme, onay kutusu, "Onayla ve kuyruğa al". */
export async function approve(page: Page, version: number) {
  await approveButton(page, version).click()
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await page.getByRole('button', { name: 'Onayla ve kuyruğa al' }).click()
}
