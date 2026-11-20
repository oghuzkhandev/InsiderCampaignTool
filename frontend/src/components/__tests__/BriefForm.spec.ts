import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import BriefForm from '../BriefForm.vue'
import { MAX_BRIEF_LENGTH } from '@/api/campaigns'
import { EXAMPLE_BRIEFS } from '@/exampleBrief'

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
    expect(ok.find('#brief-count').classes()).not.toContain('text-destructive')
    expect(over.find('#brief-count').classes()).toContain('text-destructive')
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

  describe('örnek brief önerileri', () => {
    const buttons = (w: ReturnType<typeof mountForm>) => w.findAll('[data-testid="example-brief"]')

    it('boş alanda 2-3 örnek önerir; her biri adını ve brief metnini gösterir', () => {
      const w = mountForm('')
      expect(w.find('[data-testid="examples"]').exists()).toBe(true)
      expect(buttons(w)).toHaveLength(EXAMPLE_BRIEFS.length)
      expect(EXAMPLE_BRIEFS.length).toBeGreaterThanOrEqual(2)
      expect(EXAMPLE_BRIEFS.length).toBeLessThanOrEqual(3)
      EXAMPLE_BRIEFS.forEach((example, i) => {
        expect(buttons(w)[i]?.text()).toContain(example.title)
        expect(buttons(w)[i]?.text()).toContain(example.brief.slice(0, 20))
      })
    })

    it('örnek listesi geçerlidir: benzersiz kimlikler, dolu alanlar, brief sınırı içinde', () => {
      const ids = EXAMPLE_BRIEFS.map((e) => e.id)
      expect(new Set(ids).size).toBe(ids.length)
      for (const example of EXAMPLE_BRIEFS) {
        expect(example.title.trim()).not.toBe('')
        expect(example.brief.trim()).not.toBe('')
        expect(Array.from(example.brief).length).toBeLessThanOrEqual(MAX_BRIEF_LENGTH)
      }
    })

    it('örnek seçmek metin alanını doldurur, göndermez ve öneriler kaybolur', async () => {
      const w = mountForm('')
      await buttons(w)[1]?.trigger('click')

      expect((w.find('textarea').element as HTMLTextAreaElement).value).toBe(
        EXAMPLE_BRIEFS[1]?.brief,
      )
      expect(submitEvents(w)).toHaveLength(0)
      expect(w.find('[data-testid="examples"]').exists()).toBe(false)
    })

    it('kullanıcı yazmaya başlayınca öneri göstermez (yazdığının üzerine yazılmaz)', () => {
      const w = mountForm('Kendi brief metnim')
      expect(w.find('[data-testid="examples"]').exists()).toBe(false)
    })

    it('yalnızca boşluktan oluşan metin boş sayılır ve öneriler görünür', () => {
      expect(mountForm('   ').find('[data-testid="examples"]').exists()).toBe(true)
    })

    it('örnek düğmeleri gönderim düğmesi değildir (type="button")', () => {
      const w = mountForm('')
      for (const b of buttons(w)) expect(b.attributes('type')).toBe('button')
    })
  })
})
