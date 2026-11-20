import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TemplatePicker from '../TemplatePicker.vue'
import { getTemplates } from '@/api/workspace'

vi.mock('@/api/workspace', () => ({ getTemplates: vi.fn<typeof getTemplates>() }))
afterEach(() => vi.clearAllMocks())

describe('TemplatePicker', () => {
  it('şablonun içeriğini ayrı etiketlerle gösterir ve seçilen üç alanı düzenleyiciye aktarır', async () => {
    const fields = {
      sms_text: 'Merhaba {{first_name}}, sepetin seni bekliyor.',
      email_subject: 'Sepetine özel fırsat',
      email_body: 'Alışverişini tamamla.',
    }
    vi.mocked(getTemplates).mockResolvedValue({
      items: [
        {
          id: 'template-1',
          name: 'Sepet',
          ...fields,
          created_at: '2026-11-20T09:00:00Z',
          updated_at: '2026-11-20T09:00:00Z',
        },
      ],
      total: 1,
      page: 1,
      limit: 10,
    })
    const w = mount(TemplatePicker, { props: { disabled: false } })
    await w.get('button').trigger('click')
    await flushPromises()
    const option = w.get('[data-testid="template-option"]')
    expect(option.get('h4').text()).toBe('Sepet')
    expect(option.text()).toContain('SMS metni')
    expect(option.text()).toContain(fields.sms_text)
    expect(option.text()).toContain('Konu')
    expect(option.text()).toContain(fields.email_subject)
    expect(option.text()).toContain('Gövde önizlemesi')
    expect(option.text()).toContain(fields.email_body)
    expect(w.text()).toContain('gönderim başlatmaz')
    await option.get('button').trigger('click')
    expect(w.emitted('select')).toEqual([[fields]])
    expect(w.find('[data-testid="template-option"]').exists()).toBe(false)
    w.unmount()
  })
})
