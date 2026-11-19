import { expect, test } from '@playwright/test'
import { approvedCampaign, createCampaign } from './helpers.ts'

// nginx üzerinden (ADR-014): `web` servisi derlenmiş Vue çıktısını servis eder ve /api'yi
// buffering kapalı proxy'ler. Bu testler tarayıcı değil, doğrudan HTTP akışı okur:
// akışın nginx tarafından BİRİKTİRİLMEDİĞİ zamanlamayla kanıtlanır.

interface SseEvent {
  name: string
  at: number // akış açıldığından beri geçen ms
}

/** Akışı okur; `until` true olana veya `maxMs` dolana kadar olayları zamanlarıyla toplar. */
async function readEvents(
  url: string,
  init: RequestInit,
  until: (events: SseEvent[]) => boolean,
  maxMs: number,
): Promise<{ events: SseEvent[]; response: Response }> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), maxMs)
  const start = Date.now()
  const response = await fetch(url, { ...init, signal: controller.signal })
  const reader = response.body?.getReader()
  if (!reader) throw new Error('yanıt gövdesi yok')
  const decoder = new TextDecoder()
  const events: SseEvent[] = []
  let buffer = ''
  try {
    while (!until(events)) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let cut: number
      while ((cut = buffer.indexOf('\n\n')) >= 0) {
        const block = buffer.slice(0, cut)
        buffer = buffer.slice(cut + 2)
        const name = /^event: (.+)$/m.exec(block)?.[1]
        if (name) events.push({ name, at: Date.now() - start })
      }
    }
  } catch (error) {
    // Süre dolunca iptal beklenir; toplanan olaylar yine de döner.
    if (!controller.signal.aborted) throw error
  } finally {
    clearTimeout(timer)
    controller.abort()
  }
  return { events, response }
}

test('dashboard akışı nginx üzerinden bekletilmeden ve saniyelik aralıkla gelir', async ({
  page,
  baseURL,
}) => {
  const id = await approvedCampaign(page) // standart ortamda worker yok: kampanya `approved` kalır

  const { events, response } = await readEvents(
    `${baseURL}/api/campaigns/${id}/events`,
    { headers: { Accept: 'text/event-stream' } },
    (e) => e.length >= 3,
    8000,
  )

  expect(response.status).toBe(200)
  expect(response.headers.get('content-type')).toContain('text/event-stream')
  expect(response.headers.has('content-length')).toBe(false) // akış, tek parça gövde değil
  expect(events.map((e) => e.name)).toEqual(['snapshot', 'snapshot', 'snapshot'])

  // Yukarıdaki isim denetimi tam üç olayı doğruladı; zamanlar buradan okunur.
  const [t0 = 0, t1 = 0, t2 = 0] = events.map((e) => e.at)
  // İlk görüntü bağlantı açılır açılmaz gelir (buffering açık olsaydı 4-8 KB dolana kadar
  // hiç gelmezdi).
  expect(t0).toBeLessThan(1500)
  // Sonrakiler yaklaşık saniyede bir: toplu değil, aralıklı.
  const gap1 = t1 - t0
  const gap2 = t2 - t1
  expect(gap1).toBeGreaterThan(600)
  expect(gap1).toBeLessThan(1800)
  expect(gap2).toBeGreaterThan(600)
  expect(gap2).toBeLessThan(1800)
})

test('içerik üretimi akışı nginx üzerinden eksiksiz ve sıralı gelir (delta..., validated)', async ({
  page,
  baseURL,
}) => {
  const id = await createCampaign(page)

  const { events, response } = await readEvents(
    `${baseURL}/api/campaigns/${id}/content`,
    { method: 'POST' },
    (e) => e.some((x) => x.name === 'validated' || x.name === 'error'),
    15000,
  )

  expect(response.status).toBe(200)
  expect(response.headers.get('content-type')).toContain('text/event-stream')
  expect(response.headers.has('content-length')).toBe(false)
  const names = events.map((e) => e.name)
  expect(names.at(-1)).toBe('validated')
  expect(names.filter((n) => n === 'delta').length).toBeGreaterThan(0)
  expect(names.every((n) => n === 'delta' || n === 'validated')).toBe(true) // hata olayı yok
})

test('derin bağlantı: kampanya ve dashboard yolları nginx tarafından index.html ile açılır', async ({
  page,
}) => {
  const id = await approvedCampaign(page)

  await page.goto(`/campaigns/${id}/delivery`) // doğrudan yükleme (sunucu tarafı yönlendirme)
  await expect(page.getByRole('heading', { level: 1, name: 'Gönderim takibi' })).toBeVisible()

  await page.goto(`/campaigns/${id}`)
  await expect(page.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 1')
})

test('önbellek başlıkları: index.html her zaman doğrulanır, karmalı çıktılar kalıcı önbelleklenir', async ({
  request,
}) => {
  const index = await request.get('/')
  expect(index.ok()).toBe(true)
  expect(index.headers()['cache-control']).toContain('no-store')

  const html = await index.text()
  const asset = /\/assets\/[^"']+\.js/.exec(html)?.[0] ?? ''
  expect(asset, 'index.html bir /assets/*.js içermeli').toMatch(/^\/assets\//)
  const js = await request.get(asset)
  expect(js.ok()).toBe(true)
  expect(js.headers()['cache-control']).toContain('immutable')

  // Sunucu sürümü sızdırılmaz.
  expect(index.headers()['server'] ?? '').not.toMatch(/\d+\.\d+/)
})
