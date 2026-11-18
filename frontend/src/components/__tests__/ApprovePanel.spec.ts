import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ApprovePanel from '../ApprovePanel.vue'

const audience = { total: 190, sms: 134, email: 150 }

function mountPanel(props: Partial<{ version: number; disabled: boolean; pending: boolean }> = {}) {
  return mount(ApprovePanel, {
    props: { version: 2, audience, disabled: false, pending: false, ...props },
  })
}

describe('ApprovePanel', () => {
  it('onay için iki adım ister: önce onay kutusu, sonra yayın', async () => {
    const w = mountPanel()
    await w.get('button.primary').trigger('click')

    expect(w.emitted('approve')).toBeUndefined()
    const text = w.get('[role="alertdialog"]').text()
    expect(text).toContain('Sürüm 2')
    expect(text).toContain('134 SMS')
    expect(text).toContain('150 e-posta')
    expect(text).toContain('değiştirilemez')

    await w.get('[role="alertdialog"] button.primary').trigger('click')
    expect(w.emitted('approve')).toHaveLength(1)
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
  })

  it('Vazgeç onay kutusunu kapatır ve olay yayınlamaz', async () => {
    const w = mountPanel()
    await w.get('button.primary').trigger('click')
    await w.get('button.secondary').trigger('click')
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(w.emitted('approve')).toBeUndefined()
  })

  it('devre dışıyken düğme kapalıdır ve nedenini söyler', () => {
    const w = mountPanel({ disabled: true })
    expect(w.get('button.primary').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('önce değişiklikleri kaydedin')
  })

  it('işlem sürerken düğmeler kapalıdır (çift tıklama koruması)', async () => {
    const w = mountPanel()
    await w.get('button.primary').trigger('click')
    await w.setProps({ pending: true })
    for (const b of w.findAll('button')) expect(b.attributes('disabled')).toBeDefined()
  })

  it('sürüm değişirse açık onay isteği kapanır', async () => {
    const w = mountPanel()
    await w.get('button.primary').trigger('click')
    await w.setProps({ version: 3 })
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
    expect(w.get('button.primary').text()).toContain('sürüm 3')
  })

  it('kaydedilmemiş değişiklik oluşunca (disabled) açık onay isteği kapanır', async () => {
    const w = mountPanel()
    await w.get('button.primary').trigger('click')
    await w.setProps({ disabled: true })
    expect(w.find('[role="alertdialog"]').exists()).toBe(false)
  })
})
