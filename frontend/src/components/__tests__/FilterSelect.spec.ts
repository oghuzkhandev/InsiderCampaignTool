import { afterEach, beforeAll, describe, expect, it } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import FilterSelect from '../FilterSelect.vue'

// Gerçek shadcn `Select` (Reka UI): açılır liste portalda ve işaretçi/ölçü API'leri gerektirir;
// jsdom'da bunlar yoktur, burada boş karşılıkları tanımlanır. Etkileşim klavyeyle yapılır (Enter ile
// açma ve seçme), bu da gerçek kullanıcının klavye yoludur.
beforeAll(() => {
  const proto = Element.prototype as unknown as Record<string, unknown>
  proto.hasPointerCapture ??= () => false
  proto.setPointerCapture ??= () => undefined
  proto.releasePointerCapture ??= () => undefined
  proto.scrollIntoView ??= () => undefined
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
})

const OPTIONS = [
  { value: 'eligible', label: 'Uygun' },
  { value: 'no_consent', label: 'İzin yok' },
  { value: 'no_address', label: 'Adres yok' },
]

let mounted: VueWrapper | null = null
afterEach(() => {
  mounted?.unmount()
  mounted = null
  document.body.innerHTML = ''
})

function mountSelect(props: { modelValue?: string; disabled?: boolean } = {}) {
  mounted = mount(FilterSelect, {
    attachTo: document.body,
    props: {
      id: 'sms',
      modelValue: '',
      options: OPTIONS,
      allLabel: 'Tümü',
      ...props,
    },
    attrs: { 'data-testid': 'sms-select' },
  })
  return mounted
}

const trigger = () => document.body.querySelector<HTMLElement>('[data-testid="sms-select"]')!
const optionTexts = () =>
  Array.from(document.body.querySelectorAll('[role="option"]')).map((o) => o.textContent?.trim())

async function openWithKeyboard() {
  trigger().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  await flushPromises()
}

async function choose(label: string) {
  const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]')).find(
    (o) => o.textContent?.trim() === label,
  )
  if (!option) throw new Error(`seçenek yok: ${label}`)
  option.focus()
  option.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
  await flushPromises()
}

describe('FilterSelect', () => {
  it('shadcn Select tetikleyicisidir (yerel <select> değil); id ve öznitelikler tetikleyiciye gider', () => {
    mountSelect()

    expect(document.body.querySelector('select')).toBeNull()
    expect(trigger().getAttribute('data-slot')).toBe('select-trigger')
    expect(trigger().getAttribute('role')).toBe('combobox')
    expect(trigger().id).toBe('sms')
  })

  it('<label for> tetikleyiciyi adlandırır (erişilebilir ad)', () => {
    const label = document.createElement('label')
    label.htmlFor = 'sms'
    label.textContent = 'SMS uygunluğu'
    document.body.appendChild(label)
    mountSelect()

    expect((trigger() as HTMLButtonElement).labels?.[0]?.textContent).toBe('SMS uygunluğu')
  })

  it('değer boşken "tümü" etiketini gösterir ve vurgulanmaz', () => {
    mountSelect({ modelValue: '' })

    expect(trigger().textContent).toContain('Tümü')
    expect(trigger().getAttribute('data-active')).toBe('false')
  })

  it('dolu değerde seçilen seçeneğin etiketini gösterir ve tetikleyiciyi vurgular', async () => {
    mountSelect({ modelValue: 'no_consent' })
    await flushPromises()

    expect(trigger().textContent).toContain('İzin yok')
    expect(trigger().getAttribute('data-active')).toBe('true')
  })

  it('klavyeyle açılır; "Tümü" ile üç seçeneği sırayla listeler', async () => {
    mountSelect()

    await openWithKeyboard()

    expect(trigger().getAttribute('aria-expanded')).toBe('true')
    expect(optionTexts()).toEqual(['Tümü', 'Uygun', 'İzin yok', 'Adres yok'])
  })

  it('bir seçenek seçilince değeri bildirir', async () => {
    const w = mountSelect()
    await openWithKeyboard()

    await choose('İzin yok')

    expect(w.emitted('update:modelValue')).toEqual([['no_consent']])
  })

  it('"Tümü" seçilince boş metin bildirir (işaretçi değer dışarı sızmaz)', async () => {
    const w = mountSelect({ modelValue: 'eligible' })
    await openWithKeyboard()

    await choose('Tümü')

    expect(w.emitted('update:modelValue')).toEqual([['']])
  })

  it('devre dışıyken açılmaz', async () => {
    mountSelect({ disabled: true })

    await openWithKeyboard()

    expect(trigger().hasAttribute('disabled')).toBe(true)
    expect(document.body.querySelectorAll('[role="option"]')).toHaveLength(0)
  })

  it('seçenek listesi tetikleyiciyle aynı kapsayıcıda değil, portalda (gövde) açılır', async () => {
    const w = mountSelect()
    await openWithKeyboard()

    expect(w.element.querySelector('[role="listbox"]')).toBeNull()
    expect(document.body.querySelector('[role="listbox"]')).not.toBeNull()
  })
})
