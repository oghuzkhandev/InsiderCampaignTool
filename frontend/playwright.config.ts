import process from 'node:process'
import { defineConfig, devices } from '@playwright/test'

// Uçtan uca testler yalnızca izole e2e ortamına karşı koşar (compose.e2e.yaml):
// repo kökünden `make e2e`. Adres açıkça verilmelidir; varsayılan yoktur, böylece
// testler yanlışlıkla geliştirme yığınına (ve geliştirme veritabanına) gitmez.
const baseURL = process.env.E2E_BASE_URL
if (!baseURL) {
  throw new Error(
    'E2E_BASE_URL gerekli. İzole e2e ortamı için repo kökünden `make e2e` çalıştırın (compose.e2e.yaml).',
  )
}
// Geliştirme yığınının portları (web, api, vite, postgres): bunlara karşı koşmak yasaktır.
const DEV_PORTS = ['5173', '5432', '8080', '8081']
if (DEV_PORTS.includes(new URL(baseURL).port)) {
  throw new Error(
    `E2E_BASE_URL (${baseURL}) geliştirme yığınına işaret ediyor. e2e testleri geliştirme veritabanına yazar; ` +
      'izole ortamı kullanın: make e2e',
  )
}

export default defineConfig({
  testDir: './e2e',
  // Ayrı Compose projelerini eşzamanlı doğrularken artifact klasörleri de ayrılabilir.
  outputDir: process.env.E2E_OUTPUT_DIR ?? 'test-results',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  forbidOnly: !!process.env.CI,
  // Her test kendi kampanyasını oluşturur ve birbirinden bağımsızdır; yine de ortak
  // dev sunucusu ve veritabanı üzerinde kararlılık için sırayla koşar.
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  use: {
    baseURL,
    actionTimeout: 15_000,
    // Başarısız testin izi (trace) test-results/ altında kalır; CI artifact olarak yüklenir.
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    // Yerelde izlemek için: E2E_HEADED=1
    headless: !process.env.E2E_HEADED,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
