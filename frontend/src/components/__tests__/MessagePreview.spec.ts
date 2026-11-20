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

  it('e-posta gövdesi her zaman açıktır: açılır bölüm ve tıklama gerekmez', () => {
    const w = mountPreview()

    expect(w.find('details').exists()).toBe(false)
    expect(w.find('summary').exists()).toBe(false)
    expect(w.get('[data-testid="preview-body"]').isVisible()).toBe(true)
    expect(w.text()).not.toContain('gövdesini göster')
  })

  it('SMS ve e-posta aynı satır düzeninde yan yana durabilir (kapsayıcı genişse iki sütun)', () => {
    const w = mountPreview()

    const columns = w.get('section[aria-label="SMS önizlemesi"]').element.parentElement!
    expect(columns.contains(w.get('section[aria-label="E-posta önizlemesi"]').element)).toBe(true)
    expect(columns.className).toContain('@[26rem]:grid-cols-2') // dar kapsayıcıda alt alta
    expect(w.classes()).toContain('@container')
  })

  it('e-posta konusu ve gövdesi aynı kartta, ince bir ayraçla ayrılır', () => {
    const w = mountPreview()

    const card = w.get('section[aria-label="E-posta önizlemesi"]').element
    const subject = w.get('[data-testid="preview-subject"]').element
    const body = w.get('[data-testid="preview-body"]').element
    const separator = card.querySelector('[role="separator"]')
    expect(separator).not.toBeNull()
    // Sıra: konu, ayraç, gövde.
    expect(
      subject.compareDocumentPosition(separator!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
    expect(separator!.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it("alanlar etiketlidir: e-postada Konu ve Gövde, SMS'te Gövde", () => {
    const w = mountPreview()

    expect(w.get('[data-testid="preview-subject-label"]').text()).toBe('Konu')
    expect(w.get('[data-testid="preview-body-label"]').text()).toBe('Gövde')
    expect(w.get('[data-testid="preview-sms-label"]').text()).toBe('Gövde')
  })

  it('etiketler metnin içine karışmaz (içerik metinleri değişmez)', () => {
    const w = mountPreview()

    expect(text(w, 'preview-sms')).not.toContain('Gövde')
    expect(text(w, 'preview-subject')).not.toContain('Konu')
    expect(text(w, 'preview-body')).not.toContain('Gövde')
  })

  it('SMS ve e-posta bölümlerine ad verir', () => {
    const w = mountPreview()
    expect(w.find('section[aria-label="SMS önizlemesi"]').exists()).toBe(true)
    expect(w.find('section[aria-label="E-posta önizlemesi"]').exists()).toBe(true)
  })
})
