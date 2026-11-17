import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import BriefForm from '../BriefForm.vue'
import { EXAMPLE_BRIEF } from '@/exampleBrief'

function mountForm(initial = '', loading = false) {
  const wrapper = mount(BriefForm, {
    props: {
      modelValue: initial,
      loading,
      'onUpdate:modelValue': (v: string | undefined) => wrapper.setProps({ modelValue: v ?? '' }),
    },
  })
  return wrapper
}

const submitEvents = (w: ReturnType<typeof mountForm>) => w.emitted('submit') ?? []

describe('BriefForm', () => {
  it('etiketli bir metin alanı ve gönder düğmesi gösterir', () => {
    const w = mountForm()
    expect(w.find('label[for="brief"]').text()).toBe('Brief')
    expect(w.find('textarea#brief').exists()).toBe(true)
    expect(w.find('button[type="submit"]').text()).toBe('Kitleyi hesapla')
  })

  it('boş brief gönderilmez ve hata gösterir', async () => {
    const w = mountForm('')
    await w.find('form').trigger('submit')
    expect(submitEvents(w)).toHaveLength(0)
    expect(w.find('[role="alert"]').text()).toBe('Brief boş olamaz.')
  })

  it('yalnızca boşluktan oluşan brief gönderilmez', async () => {
    const w = mountForm('   \n\t ')
    await w.find('form').trigger('submit')
    expect(submitEvents(w)).toHaveLength(0)
    expect(w.find('[role="alert"]').exists()).toBe(true)
  })

  it('yazmaya başlayınca boş hatası kaybolur', async () => {
    const w = mountForm('')
    await w.find('form').trigger('submit')
    await w.find('textarea').setValue('Bursa')
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })

  it('kırpılmış brief ile submit olayı yayar', async () => {
    const w = mountForm('  Bursa sepet  ')
    await w.find('form').trigger('submit')
    expect(submitEvents(w)).toEqual([['Bursa sepet']])
  })

  it('Ctrl+Enter ile gönderir', async () => {
    const w = mountForm('Bursa')
    await w.find('textarea').trigger('keydown', { key: 'Enter', ctrlKey: true })
    expect(submitEvents(w)).toEqual([['Bursa']])
  })

  it('karakter sayısını Unicode karakter olarak sayar', () => {
    const w = mountForm('😀İ')
    expect(w.find('#brief-count').text()).toBe('2 / 1000')
  })

  it('1000 karakter kabul edilir, 1001 reddedilir', async () => {
    const ok = mountForm('ü'.repeat(1000))
    await ok.find('form').trigger('submit')
    expect(submitEvents(ok)).toHaveLength(1)
    expect(ok.find('button[type="submit"]').attributes('disabled')).toBeUndefined()

    const over = mountForm('ü'.repeat(1001))
    expect(over.find('#brief-count').classes()).toContain('over')
    expect(over.find('[role="alert"]').text()).toContain('en fazla 1000 karakter')
    expect(over.find('button[type="submit"]').attributes('disabled')).toBeDefined()
    await over.find('form').trigger('submit')
    expect(submitEvents(over)).toHaveLength(0)
  })

  it('yüklenirken düğme devre dışı ve metni değişir', () => {
    const w = mountForm('Bursa', true)
    const button = w.find('button[type="submit"]')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.text()).toBe('Hesaplanıyor…')
  })

  it('örnek brief düğmesi metin alanını doldurur', async () => {
    const w = mountForm('')
    await w.find('button.link').trigger('click')
    expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe(EXAMPLE_BRIEF)
  })
})
