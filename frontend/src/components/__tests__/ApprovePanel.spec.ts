import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ApprovePanel from '../ApprovePanel.vue'

const audience = { total: 190, sms: 134, email: 150 }
const content = {
  sms_text: 'Merhaba {{first_name}}, %30 indirim seni bekliyor!',
  email_subject: '{{first_name}}, sepetin seni bekliyor',
  email_body: 'Merhaba {{first_name}},\n\nSepetindeki ürünler hâlâ seni bekliyor.',
}

function mountPanel(
  props: Partial<{
    version: number
    disabled: boolean
    pending: boolean
    content: typeof content
  }> = {},
) {
  return mount(ApprovePanel, {
    props: { version: 2, audience, content, disabled: false, pending: false, ...props },
  })
}

describe('onay özeti', () => {
  it('hangi sürümün onaylanacağını söyler', () => {
    const w = mountPanel({ version: 7 })
    expect(w.get('[data-testid="approve-version"]').text()).toBe('Sürüm 7 onaylanacak.')
    expect(w.get('[data-testid="approve-open"]').text()).toBe('Onayla (sürüm 7)')
  })

  it('tahmini alıcı sayılarını gösterir ve tahmin olduğunu belirtir', () => {
    const w = mountPanel()
    expect(w.get('[data-testid="approve-sms"]').text()).toBe('≈ 134')
    expect(w.get('[data-testid="approve-email"]').text()).toBe('≈ 150')
    const summary = w.get('[data-testid="approve-recipients"]').text()
    expect(summary).toContain('Tahmini SMS alıcısı')
    expect(summary).toContain('Tahmini e-posta alıcısı')
    expect(w.text()).toContain('kesin sayı onay anındaki izin ve adres durumuna göre belirlenir')
  })

  it('SMS ve e-posta önizlemelerini örnek adla, aynı kartta gösterir', () => {
    const w = mountPanel()
    expect(w.get('[data-testid="preview-sms"]').text()).toBe(
      'Merhaba Ayşe, %30 indirim seni bekliyor!',
    )
    expect(w.get('[data-testid="preview-subject"]').text()).toBe('Ayşe, sepetin seni bekliyor')
    expect(w.get('[data-testid="preview-note"]').text()).toContain('Örnek önizleme')
    expect(w.text()).not.toContain('{{first_name}}, sepetin')
  })

  it('önizleme onaylanacak sürümün metnini gösterir (sürüm değişince güncellenir)', async () => {
    const w = mountPanel()
    await w.setProps({
      version: 3,
      content: { ...content, sms_text: 'Yeni sürüm metni {{first_name}}' },
    })
    expect(w.get('[data-testid="preview-sms"]').text()).toBe('Yeni sürüm metni Ayşe')
    expect(w.get('[data-testid="approve-version"]').text()).toBe('Sürüm 3 onaylanacak.')
  })

  it('işlem sürerken düğme "Onaylanıyor…" der ve kapalıdır', async () => {
    const w = mountPanel()
    await w.setProps({ pending: true })
    const open = w.get('[data-testid="approve-open"]')
    expect(open.text()).toBe('Onaylanıyor…')
    expect(open.attributes('disabled')).toBeDefined()
  })
})

describe('ApprovePanel', () => {
  it('onay için iki adım ister: önce onay kutusu, sonra yayın', async () => {
    const w = mountPanel()
    await w.get('[data-testid="approve-open"]').trigger('click')

    expect(w.emitted('approve')).toBeUndefined()
    const text = w.get('[role="alertdialog"]').text()
    expect(text).toContain('Sürüm 2')
    expect(text).toContain('134 SMS')
    expect(text).toContain('150 e-posta')
    expect(text).toContain('değiştirilemez')

    await w.get('[data-testid="approve-confirm"]').trigger('click')
    expect(w.emitted('approve')).toHaveLength(1)
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
  })

  it('Vazgeç onay kutusunu kapatır ve olay yayınlamaz', async () => {
    const w = mountPanel()
    await w.get('[data-testid="approve-open"]').trigger('click')
    await w.get('[data-testid="approve-cancel"]').trigger('click')
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(w.emitted('approve')).toBeUndefined()
  })

  it('devre dışıyken düğme kapalıdır ve nedenini söyler', () => {
    const w = mountPanel({ disabled: true })
    expect(w.get('[data-testid="approve-open"]').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('önce değişiklikleri kaydedin')
  })

  it('işlem sürerken düğmeler kapalıdır (çift tıklama koruması)', async () => {
    const w = mountPanel()
    await w.get('[data-testid="approve-open"]').trigger('click')
    await w.setProps({ pending: true })
    for (const b of w.findAll('button')) expect(b.attributes('disabled')).toBeDefined()
  })

  it('sürüm değişirse açık onay isteği kapanır', async () => {
    const w = mountPanel()
    await w.get('[data-testid="approve-open"]').trigger('click')
    await w.setProps({ version: 3 })
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(w.get('[data-testid="approve-open"]').text()).toContain('sürüm 3')
  })

  it('kaydedilmemiş değişiklik oluşunca (disabled) açık onay isteği kapanır', async () => {
    const w = mountPanel()
    await w.get('[data-testid="approve-open"]').trigger('click')
    await w.setProps({ disabled: true })
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
  })
})
