import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import TemplatesView from '../TemplatesView.vue'
import { getTemplates, type saveTemplate, type deleteTemplate } from '@/api/workspace'

vi.mock('@/api/workspace', () => ({
  getTemplates: vi.fn<typeof getTemplates>(),
  saveTemplate: vi.fn<typeof saveTemplate>(),
  deleteTemplate: vi.fn<typeof deleteTemplate>(),
}))

let wrapper: VueWrapper | undefined
beforeEach(() => {
  vi.mocked(getTemplates).mockResolvedValue({
    items: [
      {
        id: 'template-1',
        name: 'Sepet',
        sms_text: 'Merhaba',
        email_subject: 'Sepetin seni bekliyor',
        email_body: 'Alışverişini tamamla.',
        created_at: '2026-11-20T09:00:00Z',
        updated_at: '2026-11-20T09:00:00Z',
      },
    ],
    total: 1,
    page: 1,
    limit: 10,
  })
})
afterEach(() => {
  wrapper?.unmount()
  vi.restoreAllMocks()
})

async function openEditor() {
  const view = mount(TemplatesView)
  wrapper = view
  await flushPromises()
  const editButton = view.findAll('button').find((button) => button.text().includes('Düzenle'))
  if (!editButton) throw new Error('Düzenle düğmesi bulunamadı')
  await editButton.trigger('click')
  return view
}

describe('şablon düzenlemeden çıkış', () => {
  it('kayıtlı şablon kartında e-posta gövdesini de gösterir', async () => {
    const view = await openEditor()
    const row = view.get('[data-testid="template-row"]')
    expect(row.text()).toContain('E-posta gövdesi')
    expect(row.get('[data-testid="template-body"]').text()).toBe('Alışverişini tamamla.')
    await view.get('#email_body').setValue('Henüz kaydedilmemiş düzenleme')
    expect(row.get('[data-testid="template-body"]').text()).toBe('Alışverişini tamamla.')
  })

  it('satırdaki Vazgeç düzenlemeyi kapatır ve yeniden açılabilir', async () => {
    const view = await openEditor()
    const row = view.get('[data-testid="template-row"]')
    expect(row.text()).toContain('Düzenleme açık')
    await row.get('button').trigger('click')
    expect(row.attributes('data-editing')).toBe('false')
    expect(view.get('#template-name').element).toHaveProperty('value', '')
    expect(view.text()).toContain('Yeni şablon')
    await row.get('button').trigger('click')
    expect(row.attributes('data-editing')).toBe('true')
    expect(view.get('#template-name').element).toHaveProperty('value', 'Sepet')
  })

  it('formdaki vazgeç düğmesi de düzenlemeyi kapatır', async () => {
    const view = await openEditor()
    const cancelButton = view
      .findAll('button')
      .find((button) => button.text().includes('Düzenlemekten vazgeç'))
    if (!cancelButton) throw new Error('Vazgeç düğmesi bulunamadı')
    await cancelButton.trigger('click')
    expect(view.get('[data-testid="template-row"]').attributes('data-editing')).toBe('false')
  })

  it('kaydedilmemiş değişiklikte iptal edilirse metni korur, onaylanırsa çıkar', async () => {
    const view = await openEditor()
    await view.get('#template-name').setValue('Yeni ad')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const row = view.get('[data-testid="template-row"]')
    await row.get('button').trigger('click')
    expect(confirm).toHaveBeenCalledOnce()
    expect(row.attributes('data-editing')).toBe('true')
    expect(view.get('#template-name').element).toHaveProperty('value', 'Yeni ad')
    confirm.mockReturnValue(true)
    await row.get('button').trigger('click')
    expect(row.attributes('data-editing')).toBe('false')
    expect(view.get('#template-name').element).toHaveProperty('value', '')
  })

  it('şablon adını ikon yerine açık bir etiketle gösterir', async () => {
    const view = await openEditor()
    expect(view.get('h3').text()).toBe('Şablon adı: Sepet')
    expect(view.get('h3').find('svg').exists()).toBe(false)
  })
})
