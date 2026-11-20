import { expect, test, type Locator, type Page } from '@playwright/test'
import {
  BRIEF,
  approve,
  approveButton,
  createCampaign,
  expectAudience,
  generateContent,
} from './helpers.ts'

// Telefon genişliğinde (360 px) ekranlar: yatay taşma yok ve ana eylemler yeterince büyük.
// Dashboard'un telefon görünümü `delivery-dashboard.spec.ts` içinde sınanır.

test.use({ viewport: { width: 360, height: 740 } })

/** Dokunma hedefi için alt sınır (px): shadcn `lg` düğme 40 px'tir. */
const MIN_TARGET = 40

/**
 * Telefonda kenar çubuğu bir çekmecedir: üst başlıktaki düğmeyle açılır ve soldan kayarak gelir
 * (200 ms). Animasyon sürerken içindeki her öğe viewport'un solunda ölçülür; bu yüzden açıldıktan
 * sonra animasyonun bitmesi beklenir. Yerelde hızlı geçtiği için gizli kalan, yavaş CI'da
 * "viewport dışına taşan öğeler" diye kırılan bir zamanlama hatasıydı. Sonsuz animasyonlar
 * (iskelet nabzı gibi) hariçtir; onların `finished` sözü hiç çözülmez.
 */
async function openSidebar(page: Page) {
  await page.getByRole('button', { name: 'Kenar çubuğunu aç veya kapat' }).click()
  await expect(page.locator('[data-slot="sidebar"][data-mobile="true"]')).toBeVisible()
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
        .map((a) => a.finished),
    ),
  )
}

async function expectNoHorizontalOverflow(page: Page, screen: string, scope = 'main') {
  const overflow = await page
    .locator('html')
    .evaluate((root) => root.scrollWidth - root.clientWidth)
  expect(overflow, `${screen}: yatay taşma (px)`).toBeLessThanOrEqual(0)

  // Kartlar `overflow-hidden` olduğu için taşan içerik belge genişliğini değiştirmez, kırpılır
  // (kullanıcı metnin bir kısmını göremez). Bu yüzden görünür her öğenin viewport içinde
  // kaldığı da denetlenir (varsayılan kapsam `main`; üst başlık ve çekmece için ayrı verilir).
  // Yalnızca ekran okuyucuya ayrılmış (`sr-only`) öğeler hariçtir.
  const outside = await page.evaluate(
    (selector) =>
      Array.from(document.querySelectorAll(selector))
        .filter((el) => el.getClientRects().length > 0 && !el.closest('.sr-only'))
        .filter((el) => {
          const bounds = el.getBoundingClientRect()
          // Duyarlı sr-only sınıfı etkin olduğunda alt öğeler de görsel olarak kırpılır.
          // Sınıf adına değil, tarayıcının uyguladığı gerçek kırpmaya bak.
          for (let parent: Element | null = el; parent; parent = parent.parentElement) {
            const style = getComputedStyle(parent)
            if (style.clip === 'rect(0px, 0px, 0px, 0px)' || style.clipPath === 'inset(50%)')
              return false
            // %0 ilerleme göstergesi kendi taşması gizli kabının tamamen dışındadır.
            // Görünmeyen bu katman belgeyi taşırmaz; kısmen görünen içerik yine denetlenir.
            if (style.overflowX === 'hidden' || style.overflowX === 'clip') {
              const clip = parent.getBoundingClientRect()
              if (bounds.right <= clip.left || bounds.left >= clip.right) return false
            }
          }
          return true
        })
        .filter((el) => {
          const r = el.getBoundingClientRect()
          return r.right > window.innerWidth + 0.5 || r.left < -0.5
        })
        .map((el) => `${el.tagName.toLowerCase()} "${(el.textContent ?? '').trim().slice(0, 30)}"`),
    `${scope} *`,
  )
  expect(outside, `${screen}: viewport dışına taşan öğeler`).toEqual([])
}

async function expectTouchTarget(target: Locator, label: string) {
  const box = await target.boundingBox()
  expect(box, `${label} görünür olmalı`).not.toBeNull()
  expect(box?.height ?? 0, `${label}: yükseklik`).toBeGreaterThanOrEqual(MIN_TARGET)
}

test('telefonda uzun brief ve hata metinleri taşmaz', async ({ page }, info) => {
  await page.goto('/')
  await expectNoHorizontalOverflow(page, 'önizleme (örnek brief önerileri)')
  await expectTouchTarget(page.getByTestId('example-brief').first(), 'örnek brief düğmesi')

  // Boşluksuz çok uzun brief: desteklenmez, hata metni brief'ten bir parça içerir.
  const unbroken = 'x'.repeat(400)
  await page.getByLabel('Brief').fill(unbroken)
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expect(page.getByTestId('error-panel')).toBeVisible()
  await expectNoHorizontalOverflow(page, 'önizleme (uzun hata metni)')
  await page.screenshot({ path: info.outputPath('mobile-long-error.png'), fullPage: true })

  // Desteklenen ama boşluksuz uzun brief: kampanya ekranında brief kartı taşmaz.
  await page.getByLabel('Brief').fill(`sepet${'a'.repeat(400)}`)
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expect(page.getByTestId('total')).toBeVisible()
  await expectNoHorizontalOverflow(page, 'önizleme (uzun brief sonucu)')
  await page.getByRole('link', { name: 'İleri: İçeriği hazırla' }).click()
  await page.waitForURL(/\/campaigns\/[0-9a-f-]{36}$/)
  await expect(page.getByRole('region', { name: 'Brief' })).toContainText('sepetaaaa')
  await expectNoHorizontalOverflow(page, 'kampanya (uzun brief)')
  await page.screenshot({ path: info.outputPath('mobile-long-brief.png'), fullPage: true })

  // Çekmecede kampanya satırları yerine sabit sayfa bağlantıları bulunur.
  await page.goto('/')
  await openSidebar(page)
  await expectNoHorizontalOverflow(page, 'çekmece', '[data-slot="sidebar"]')
  await expectTouchTarget(page.getByTestId('sidebar-new-campaign'), 'Yeni kampanya')
  await page.getByRole('link', { name: 'Kampanyalar', exact: true }).click()
  await page.waitForURL((url) => url.pathname === '/campaigns')
  await expect(page.locator('[data-slot="sidebar"][data-mobile="true"]')).toHaveCount(0)
  await expectNoHorizontalOverflow(page, 'kampanya listesi')
})

test('telefonda üst başlık taşmaz; "Insider Campaign Tool" ana sayfaya döner', async ({ page }) => {
  await createCampaign(page)
  await expectNoHorizontalOverflow(page, 'üst başlık', 'header')

  const brand = page.getByTestId('brand-link')
  await expect(brand).toHaveText('Insider Campaign Tool')
  await expectTouchTarget(brand, 'Campaign Tool başlığı')
  await brand.click()
  await page.waitForURL((url) => url.pathname === '/overview')
  await expect(page.getByRole('heading', { level: 1, name: 'Genel bakış' })).toBeVisible()
})

test('telefonda önizleme ve kampanya ekranları taşmaz, ana eylemler büyüktür', async ({
  page,
}, info) => {
  await page.goto('/')
  await expectNoHorizontalOverflow(page, 'önizleme (boş)')
  await expectTouchTarget(page.getByRole('button', { name: 'Kitleyi hesapla' }), 'Kitleyi hesapla')

  await page.getByLabel('Brief').fill(BRIEF)
  await page.getByRole('button', { name: 'Kitleyi hesapla' }).click()
  await expectAudience(page)
  await expectNoHorizontalOverflow(page, 'önizleme (sonuç)')
  await expectTouchTarget(page.getByRole('link', { name: 'İçeriği hazırla' }), 'İçeriği hazırla')
  await page.screenshot({ path: info.outputPath('mobile-preview.png'), fullPage: true })

  await createCampaign(page)
  await expectNoHorizontalOverflow(page, 'kampanya (içeriksiz)')
  await expectTouchTarget(page.getByRole('button', { name: 'İçerik üret' }), 'İçerik üret')

  await generateContent(page)
  await expectNoHorizontalOverflow(page, 'kampanya (içerikli)')
  await expectTouchTarget(
    page.getByRole('button', { name: 'Yeni sürüm olarak kaydet' }),
    'Yeni sürüm olarak kaydet',
  )
  await expectTouchTarget(approveButton(page, 1), 'Onayla')
  await page.screenshot({ path: info.outputPath('mobile-campaign.png'), fullPage: true })

  await approveButton(page, 1).click()
  await expect(page.getByRole('alertdialog')).toBeVisible()
  await expectNoHorizontalOverflow(page, 'onay penceresi')
  await expectTouchTarget(
    page.getByRole('button', { name: 'Onayla ve kuyruğa al' }),
    'Onayla ve kuyruğa al',
  )
  await page.getByRole('button', { name: 'Vazgeç' }).click()

  await approve(page, 1)
  await expect(page).toHaveURL(/\/campaigns\/[0-9a-f-]{36}\/delivery$/)
  await expectNoHorizontalOverflow(page, 'otomatik açılan gönderim takibi')
  await expectTouchTarget(
    page.getByRole('link', { name: 'Geri: Kampanyaya dön' }),
    'Kampanyaya dön',
  )
  await page.screenshot({ path: info.outputPath('mobile-delivery.png'), fullPage: true })
})
