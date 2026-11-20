import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MessagePreview from '../MessagePreview.vue'
import { SAMPLE_NAME } from '@/content/draft'

const fields = {
  sms_text: 'Merhaba {{first_name}}, %30 indirim seni bekliyor!',
  email_subject: '{{first_name}}, sepetin seni bekliyor',
  email_body: 'Merhaba {{first_name}},\n\nSepetindeki ürünler hâlâ seni bekliyor.',
}

const mountPreview = (props: Record<string, unknown> = {}) =>
  mount(MessagePreview, { props: { fields, ...props } })
const text = (w: ReturnType<typeof mountPreview>, id: string) =>
  w.get(`[data-testid="${id}"]`).text()

describe('MessagePreview', () => {
  it('{{first_name}} yerine örnek ad koyar (SMS, konu ve gövde)', () => {
    const w = mountPreview()
    expect(text(w, 'preview-sms')).toBe(`Merhaba ${SAMPLE_NAME}, %30 indirim seni bekliyor!`)
    expect(text(w, 'preview-subject')).toBe(`${SAMPLE_NAME}, sepetin seni bekliyor`)
    expect(text(w, 'preview-body')).toContain(`Merhaba ${SAMPLE_NAME},`)
    expect(w.text()).not.toContain('Merhaba {{first_name}}')
  })

  it('bunun örnek olduğunu ve gerçek mesajların alıcının adıyla gittiğini açıkça belirtir', () => {
    const note = text(mountPreview(), 'preview-note')
    expect(note).toContain('Örnek önizleme')
    expect(note).toContain(`"${SAMPLE_NAME}"`)
    expect(note).toContain('{{first_name}}')
    expect(note).toContain('her alıcının kendi adıyla')
  })

  it('metni HTML olarak yorumlamaz', () => {
    const w = mountPreview({
      fields: { ...fields, email_body: '<img src=x onerror=alert(1)> {{first_name}}' },
    })
    expect(w.find('img').exists()).toBe(false)
    expect(text(w, 'preview-body')).toContain(`<img src=x onerror=alert(1)> ${SAMPLE_NAME}`)
  })

  it('boş alanlarda tire gösterir', () => {
    const w = mountPreview({ fields: { sms_text: '', email_subject: '  ', email_body: '' } })
    expect(text(w, 'preview-sms')).toBe('—')
    expect(text(w, 'preview-body')).toBe('—')
  })

  it('kompakt görünümde gövde açılır bölümdedir, normalde doğrudan görünür', () => {
    expect(mountPreview().find('details').exists()).toBe(false)
    const compact = mountPreview({ compact: true })
    expect(compact.find('details').exists()).toBe(true)
    expect(compact.get('details summary').text()).toBe('E-posta gövdesini göster')
    expect(compact.find('details [data-testid="preview-body"]').exists()).toBe(true)
  })

  it('SMS ve e-posta bölümlerine ad verir', () => {
    const w = mountPreview()
    expect(w.find('section[aria-label="SMS önizlemesi"]').exists()).toBe(true)
    expect(w.find('section[aria-label="E-posta önizlemesi"]').exists()).toBe(true)
  })
})
