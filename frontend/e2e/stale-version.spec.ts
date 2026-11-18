import { expect, test } from '@playwright/test'
import {
  approve,
  approveButton,
  createCampaign,
  EXPECTED,
  generateContent,
  saveButton,
  serverState,
  smsField,
} from './helpers.ts'

test('iki sekme: eski sürümü görüp onaylamak reddedilir, son sürüm yüklenir', async ({
  page: pageA,
  context,
}) => {
  // Sekme A: kampanyayı oluşturur ve içeriği üretir (sürüm 1).
  const id = await createCampaign(pageA)
  await generateContent(pageA)

  // Sekme B: aynı kampanyayı açar ve sürüm 1'i görür.
  const pageB = await context.newPage()
  await pageB.goto(`/campaigns/${id}`)
  await expect(pageB.getByTestId('version')).toHaveText('Sürüm 1 · Yapay zekâ ile üretildi')
  await expect(approveButton(pageB, 1)).toBeEnabled()

  // Sekme A: düzenleyip kaydeder (sürüm 2).
  const edited = 'Merhaba {{first_name}}, A sekmesinde düzenlendi: %30 indirim!'
  await smsField(pageA).fill(edited)
  await saveButton(pageA).click()
  await expect(pageA.getByTestId('version')).toHaveText('Sürüm 2 · Elle düzenlendi')

  // Sekme B hâlâ sürüm 1'i görürken onaylamayı dener: reddedilir.
  await approve(pageB, 1)
  await expect(pageB.getByText('içerik sürümü güncel değil')).toBeVisible()

  // Son sürüm yüklenir ve kendiliğinden onaylanmaz.
  await expect(pageB.getByTestId('version')).toHaveText('Sürüm 2 · Elle düzenlendi')
  await expect(smsField(pageB)).toHaveValue(edited)
  await expect(pageB.getByTestId('approved')).toHaveCount(0)
  const state = await serverState(pageB, id)
  expect(state.status).toBe('draft') // sunucuda onay yok, mesaj kuyruğa girmedi
  expect(state.approved_content_version).toBeUndefined()

  // Güncel sürümü görüp onaylamak çalışır (kullanıcı kaldığı yerden devam eder).
  await approve(pageB, 2)
  await expect(pageB.getByTestId('approved')).toContainText('Kampanya onaylandı: sürüm 2')
  await expect(pageB.getByTestId('queued')).toContainText(
    `${EXPECTED.sms} SMS ve ${EXPECTED.email} e-posta mesajı`,
  )
})
