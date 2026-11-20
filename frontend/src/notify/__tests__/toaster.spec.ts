import { afterEach, describe, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { toast } from 'vue-sonner'
import { Toaster } from '@/components/ui/sonner'
import { notify } from '../notify'

// Gerçek `Toaster` ile uçtan uca: notify çağrısı DOM'da bildirim kartı olarak görünür ve kapatılınca
// kaybolur. (Birim testleri kartı ve notify'ı ayrı ayrı, sahte toast ile sınar.)
let wrapper: VueWrapper | null = null
afterEach(() => {
  toast.dismiss()
  wrapper?.unmount()
  wrapper = null
  document.body.innerHTML = ''
})

const cards = () => Array.from(document.body.querySelectorAll('[data-testid="notification"]'))

async function mountToaster() {
  wrapper = mount(Toaster, { attachTo: document.body, props: { position: 'top-center' } })
  await flushPromises()
}

describe('Toaster ile bildirimler', () => {
  it('notify çağrısı kartı gösterir: tür, başlık, açıklama ve 8 sn sayacı', async () => {
    await mountToaster()

    notify.success('Şablon kaydedildi', 'Kampanyada kullanabilirsiniz.')
    await flushPromises()

    expect(cards()).toHaveLength(1)
    const card = cards()[0]!
    expect(card.getAttribute('data-kind')).toBe('success')
    expect(card.textContent).toContain('Şablon kaydedildi')
    expect(card.textContent).toContain('Kampanyada kullanabilirsiniz.')
    expect(card.querySelector('[data-testid="notification-time"]')?.textContent?.trim()).toBe(
      '8 sn',
    )
  })

  it('birden çok eylem birden çok bildirim üretir (hata ve başarı birlikte)', async () => {
    await mountToaster()

    notify.error('İçerik kaydedilemedi', 'Sunucuya ulaşılamadı.')
    notify.success('Şablon silindi')
    await flushPromises()

    const kinds = cards().map((c) => c.getAttribute('data-kind'))
    expect(kinds.sort()).toEqual(['error', 'success'])
  })

  it('kapatma düğmesi kartı kaldırır', async () => {
    await mountToaster()
    notify.warning('Dikkat', 'Kaydedilmemiş değişiklik var.')
    await flushPromises()

    ;(
      document.body.querySelector('[data-testid="notification-close"]') as HTMLButtonElement
    ).click()
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 600)) // çıkış animasyonu

    expect(cards()).toHaveLength(0)
  })
})
