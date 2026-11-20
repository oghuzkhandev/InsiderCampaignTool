import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { localDateTime } from '@/campaign/schedule'
import ApprovePanel from '../ApprovePanel.vue'

const audience = { total: 190, sms: 134, email: 150 }

function mountPanel(
  props: Partial<{
    version: number
    disabled: boolean
    pending: boolean
  }> = {},
) {
  return mount(ApprovePanel, {
    props: { version: 2, audience, disabled: false, pending: false, ...props },
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

  it('önizlemeyi içermez ve onaylanacak sürüm değişince özeti günceller', async () => {
    const w = mountPanel()
    expect(w.find('[data-testid="message-preview"]').exists()).toBe(false)
    await w.setProps({ version: 3 })
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
  it('gönderim zamanı ve onay düğmesi alıcılarla aynı kartta, önizleme ise dışarıdadır', () => {
    const w = mountPanel()
    const schedule = w.get('[data-testid="approve-schedule"]')
    expect(schedule.findAll('[role="radio"]')).toHaveLength(2)
    expect(schedule.find('input[type="radio"]').exists()).toBe(false)
    expect(schedule.find('[aria-label="Hemen gönder hakkında bilgi"]').exists()).toBe(true)
    expect(schedule.find('[aria-label="Tarih ve saat seç hakkında bilgi"]').exists()).toBe(true)
    const summary = w.get('[aria-labelledby="approve-title"]')
    expect(summary.find('[data-testid="approve-schedule"]').exists()).toBe(true)
    expect(summary.find('[data-testid="approve-open"]').exists()).toBe(true)
    expect(summary.find('[data-testid="message-preview"]').exists()).toBe(false)
    expect(schedule.text()).toContain('daha erken gönderilmez')
  })

  it('tarih seçimi boş veya geçmişse onay diyaloğunu açmaz', async () => {
    const w = mountPanel()
    await w.get('#send-later').trigger('click')
    await w.get('[data-testid="approve-open"]').trigger('click')
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(w.get('#send-time-error').text()).toContain('tarih ve saat')
    await w.get('#send-time').setValue('2020-01-01T12:00')
    await w.get('[data-testid="approve-open"]').trigger('click')
    expect(w.get('#send-time-error').text()).toContain('gelecekte')
    expect(w.emitted('approve')).toBeUndefined()
  })

  it('gelecekteki yerel saati özetler ve yalnızca son onayda UTC olarak yayınlar', async () => {
    const w = mountPanel()
    const at = new Date(2035, 4, 12, 14, 30)
    await w.get('#send-later').trigger('click')
    await w.get('#send-time').setValue(localDateTime(at))
    await w.get('[data-testid="approve-open"]').trigger('click')
    expect(w.get('[data-testid="confirm-schedule"]').text()).toContain('2035')
    expect(w.emitted('approve')).toBeUndefined()
    await w.get('[data-testid="approve-confirm"]').trigger('click')
    expect(w.emitted('approve')).toEqual([[at.toISOString()]])
  })

  it('onay diyaloğundayken saat değiştirilirse önceki onay kapanır', async () => {
    const w = mountPanel()
    await w.get('#send-later').trigger('click')
    await w.get('#send-time').setValue('2035-05-12T14:30')
    await w.get('[data-testid="approve-open"]').trigger('click')
    await w.get('#send-time').setValue('2035-05-12T15:30')
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
  })
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
