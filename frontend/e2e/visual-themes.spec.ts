import { expect, test, type Locator, type Page, type TestInfo } from '@playwright/test'
import { approveButton, approvedCampaign, createCampaign, generateContent } from './helpers.ts'

// Açık ve koyu tema (işletim sistemi ayarı) ile ana ekranlar:
//  * okunabilirlik: metin ile gerçek arka planı arasındaki WCAG kontrastı,
//  * her tıklanabilir öğe `cursor: pointer` taşır (devre dışı olanlar hariç),
//  * ekran görüntüleri test çıktısına (test-results/) yazılır; gözle bakmak için.
//
// Regresyon: shadcn token'ları yalnızca `.dark` sınıfında koyulaşıyordu ve sınıf hiç
// eklenmiyordu; işletim sistemi koyu temadayken beyaz kartlar ve koyu zeminde koyu
// yazılı rozetler çıkıyordu. Kampanya ve onay gerçek backend ile; yalnızca dashboard
// olay akışı taklit edilir.

type Rgb = [number, number, number]

/** Bir öğenin metin rengi ve gerçek (saydam olmayan ilk atadaki) arka plan rengi, sRGB 0-255. */
async function colorsOf(target: Locator): Promise<{ fg: Rgb; bg: Rgb }> {
  // Tarayıcıda çalışır; canvas her CSS renk biçimini (oklch dahil) sRGB'ye çevirir.
  return target.evaluate((el): { fg: Rgb; bg: Rgb } => {
    const canvas = document.createElement('canvas')
    canvas.width = 1
    canvas.height = 1
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) throw new Error('canvas bağlamı yok')
    const toRgba = (css: string): [number, number, number, number] => {
      ctx.clearRect(0, 0, 1, 1)
      ctx.fillStyle = css
      ctx.fillRect(0, 0, 1, 1)
      const [r = 0, g = 0, b = 0, a = 0] = ctx.getImageData(0, 0, 1, 1).data
      return [r, g, b, a / 255]
    }
    const [fr, fg, fb] = toRgba(getComputedStyle(el).color)
    // Gerçek zemin: saydam olmayan ilk ata (kart, gövde).
    let background: [number, number, number, number] | null = null
    for (let node: Element | null = el; node && !background; node = node.parentElement) {
      const candidate = toRgba(getComputedStyle(node).backgroundColor)
      if (candidate[3] > 0.99) background = candidate
    }
    const [br, bgc, bb] = background ?? toRgba(getComputedStyle(document.body).backgroundColor)
    return { fg: [fr, fg, fb], bg: [br, bgc, bb] }
  })
}

function luminance([r, g, b]: Rgb): number {
  const lin = (v: number) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

function contrast(a: Rgb, b: Rgb): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

/** Metin ile arka planı arasındaki kontrast en az `min` olmalı (WCAG AA metin: 4,5). */
async function expectReadable(target: Locator, label: string, min = 4.5) {
  await expect(target.first(), `${label} görünür olmalı`).toBeVisible()
  const { fg, bg } = await colorsOf(target.first())
  const ratio = contrast(fg, bg)
  expect(
    ratio,
    `${label}: kontrast ${ratio.toFixed(2)} (metin ${fg}, zemin ${bg})`,
  ).toBeGreaterThanOrEqual(min)
}

/** Sayfadaki görünür ve etkin tıklanabilir öğelerin imleci. */
async function actionableCursors(page: Page): Promise<{ name: string; cursor: string }[]> {
  return page.evaluate(() =>
    Array.from(
      document.querySelectorAll<HTMLElement>(
        'button, a[href], [role=button], [role=checkbox], summary',
      ),
    )
      .filter(
        (el) =>
          !(el instanceof HTMLButtonElement && el.disabled) &&
          el.getAttribute('aria-disabled') !== 'true' &&
          el.getClientRects().length > 0,
      )
      .map((el) => ({
        name: (el.getAttribute('aria-label') ?? el.textContent ?? el.tagName).trim().slice(0, 50),
        cursor: getComputedStyle(el).cursor,
      })),
  )
}

async function expectPointerEverywhere(page: Page, screen: string) {
  const found = await actionableCursors(page)
  expect(found.length, `${screen}: tıklanabilir öğe bulunmalı`).toBeGreaterThan(0)
  const wrong = found.filter((f) => f.cursor !== 'pointer')
  expect(wrong, `${screen}: imleci el olmayan tıklanabilir öğeler`).toEqual([])
}

async function shot(page: Page, info: TestInfo, name: string) {
  const path = info.outputPath(`${name}.png`)
  await page.screenshot({ path, fullPage: true })
  await info.attach(name, { path, contentType: 'image/png' })
}

function fulfillSnapshot(status: string, exhausted: number) {
  const body = {
    status,
    messages: { queued: 0, retrying: 0, sending: 0, sent: 5, failed: exhausted, skipped: 1 },
    failures: { exhausted, rejected: 0 },
  }
  return {
    status: 200,
    contentType: 'text/event-stream',
    headers: { 'Cache-Control': 'no-store' },
    body: `event: snapshot\ndata: ${JSON.stringify(body)}\n\n`,
  }
}

for (const scheme of ['light', 'dark'] as const) {
  test.describe(`${scheme} tema`, () => {
    test.use({ colorScheme: scheme })

    test('kampanya ekranı: okunabilir ve tıklanabilir her şey el imleci taşır', async ({
      page,
    }, info) => {
      await createCampaign(page)
      await shot(page, info, `${scheme}-campaign-brief-result`)

      await generateContent(page)
      await expectReadable(page.getByRole('heading', { level: 1 }), 'başlık')
      await expectReadable(approveButton(page, 1), 'Onayla düğmesi')
      await expectReadable(
        page.getByRole('button', { name: 'Yeniden üret' }),
        'Yeniden üret düğmesi',
      )
      await expectPointerEverywhere(page, 'kampanya (taslak içerik)')
      await shot(page, info, `${scheme}-campaign-content`)

      // Onay penceresi (alertdialog): kart rengi, düğmeler ve onay kutusu.
      await approveButton(page, 1).click()
      const dialog = page.getByRole('alertdialog')
      await expect(dialog).toBeVisible()
      await expectReadable(dialog, 'onay penceresi')
      await expectPointerEverywhere(page, 'onay penceresi')
      await shot(page, info, `${scheme}-approve-dialog`)
      await page.getByRole('button', { name: 'Onayla ve kuyruğa al' }).click()
      await expect(page.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 1')
      await expectReadable(page.getByTestId('approved'), 'onay bilgisi')
      await shot(page, info, `${scheme}-campaign-approved`)
    })

    test('dashboard: hata, tamamlandı ve notlar okunabilir; tıklanabilir her şey el imleci taşır', async ({
      page,
    }, info) => {
      const id = await approvedCampaign(page)

      // Başlatılamayan takip (yeniden denenmeyen hata): kırmızı uyarı ve "Yeniden bağlan".
      await page.route(`**/api/campaigns/${id}/events`, (route) =>
        route.fulfill({
          status: 403,
          contentType: 'application/json',
          body: JSON.stringify({
            error: { code: 'FORBIDDEN', message: 'Bu kampanyanın takibine erişilemiyor.' },
          }),
        }),
      )
      await page.getByRole('link', { name: 'Gönderimi izle' }).click()
      await page.waitForURL(new RegExp(`/campaigns/${id}/delivery$`))
      const failed = page.getByTestId('connection-failed')
      await expect(failed).toBeVisible()
      await expectReadable(failed, 'bağlantı hatası uyarısı')
      await expectReadable(
        page.getByRole('button', { name: 'Yeniden bağlan' }),
        'Yeniden bağlan düğmesi',
      )
      await expectPointerEverywhere(page, 'dashboard (bağlantı hatası)')
      await shot(page, info, `${scheme}-dashboard-error`)

      // Tamamlanmış gönderim, kısmen başarısız.
      await page.unroute(`**/api/campaigns/${id}/events`)
      await page.route(`**/api/campaigns/${id}/events`, (route) =>
        route.fulfill(fulfillSnapshot('completed', 1)),
      )
      await page.reload()
      await expect(page.getByTestId('campaign-status')).toHaveText('Tamamlandı')
      await expectReadable(page.getByTestId('campaign-status'), 'durum rozeti')
      await expectReadable(page.getByTestId('total-messages'), 'toplam mesaj')
      await expectReadable(page.getByTestId('count-sent'), 'gönderilen sayısı')
      await expectReadable(page.getByTestId('note-exhausted'), 'başarısızlık notu')
      await expectReadable(page.getByTestId('completed-note'), 'tamamlandı notu')
      await expectReadable(page.getByTestId('connection-status'), 'bağlantı durumu')
      await expectPointerEverywhere(page, 'dashboard (tamamlandı)')
      await shot(page, info, `${scheme}-dashboard-completed`)
    })
  })
}
