import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { ContentFields } from '@/api/content'
import ContentEditor from '../ContentEditor.vue'

const fields = {
  sms_text: 'Merhaba {{first_name}}!',
  email_subject: 'Konu',
  email_body: 'Gövde',
}

function mountEditor(props: Record<string, unknown> = {}, modelValue = fields) {
  return mount(ContentEditor, { props: { modelValue, readonly: false, ...props } })
}

const counter = (w: ReturnType<typeof mountEditor>, id: string) => w.get(`#${id}`)

describe('ContentEditor', () => {
  it('üç alanı doldurur', () => {
    const w = mountEditor()
    expect((w.get('#sms_text').element as HTMLTextAreaElement).value).toBe(fields.sms_text)
    expect((w.get('#email_subject').element as HTMLInputElement).value).toBe('Konu')
    expect((w.get('#email_body').element as HTMLTextAreaElement).value).toBe('Gövde')
  })

  it('sayaçta {{first_name}} yerine fallback uzunluğunu varsayar, kod noktası sayar', async () => {
    const w = mountEditor()
    // "Merhaba !" 9 + "Değerli müşterimiz" 18
    expect(counter(w, 'sms-count').text()).toBe('27 / 160')
    expect(counter(w, 'subject-count').text()).toBe('4 / 80')

    await w.setProps({ modelValue: { ...fields, sms_text: '😀'.repeat(160) } })
    expect(counter(w, 'sms-count').text()).toBe('160 / 160')
    expect(counter(w, 'sms-count').classes()).not.toContain('text-destructive')
  })

  it('sınır aşılınca sayacı uyarı olarak işaretler ama yazmayı engellemez', async () => {
    const w = mountEditor()
    await w.setProps({
      modelValue: { ...fields, sms_text: 'a'.repeat(161), email_subject: 'b'.repeat(81) },
    })
    expect(counter(w, 'sms-count').classes()).toContain('text-destructive')
    expect(counter(w, 'subject-count').classes()).toContain('text-destructive')
    expect(w.get('#sms_text').attributes('aria-invalid')).toBe('true')
    expect(w.get('#sms_text').attributes('readonly')).toBeUndefined()
  })

  it('yazınca güncellenmiş modeli yayınlar (diğer alanlar korunur)', async () => {
    const w = mountEditor()
    await w.get('#email_subject').setValue('Yeni konu')
    await w.get('#sms_text').setValue('Yeni SMS')
    const events = w.emitted('update:modelValue') as unknown as ContentFields[][]
    expect(events[0]![0]).toEqual({ ...fields, email_subject: 'Yeni konu' })
    // defineModel yerel değeri tutar: ikinci olay ilk düzenlemeyi de içerir.
    expect(events[1]![0]).toEqual({ ...fields, email_subject: 'Yeni konu', sms_text: 'Yeni SMS' })
  })

  it('readonly iken alanlar yalnızca okunurdur', () => {
    const w = mountEditor({ readonly: true })
    for (const id of ['sms_text', 'email_subject', 'email_body']) {
      expect(w.get(`#${id}`).attributes('readonly')).toBeDefined()
    }
  })

  it('doğrulama mesajlarını ilgili alanın altında gösterir', () => {
    const w = mountEditor({
      problems: {
        sms_text: ['SMS metni 160 karakteri aşıyor.'],
        email_body: ['Gövde boş olamaz.'],
      },
    })
    expect(w.get('#sms-problems').text()).toContain('SMS metni 160')
    expect(w.get('#body-problems').text()).toContain('Gövde boş')
    expect(w.get('#subject-problems').text()).toBe('')
    expect(w.get('#email_body').attributes('aria-invalid')).toBe('true')
  })

  it('metni HTML olarak yorumlamaz', () => {
    const w = mountEditor({}, { ...fields, email_body: '<img src=x onerror=alert(1)>' })
    expect(w.find('img').exists()).toBe(false)
    expect((w.get('#email_body').element as HTMLTextAreaElement).value).toContain('<img')
  })
})
