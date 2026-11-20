import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, type PropType } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { flushPromises, mount, RouterLinkStub, type VueWrapper } from '@vue/test-utils'
import CampaignListView from '../CampaignListView.vue'
import { jsonResponse } from '@/__tests__/helpers'

// Kampanyalar sayfasının durum süzgeci artık yerel <select> değil, Kitleler'deki gibi shadcn `Select`
// (FilterSelect). Etkileşim testlerinde FilterSelect, aynı arayüzlü yerel bir seçiciyle değiştirilir
// (gerçek bileşen `FilterSelect.spec.ts`'te sınanır).
const fetchMock = vi.fn<typeof fetch>()
const row = {
  id: 'c1',
  brief: 'Örnek brief',
  status: 'draft',
  created_at: '2026-11-20T09:00:00Z',
}

const FilterSelectStub = defineComponent({
  name: 'FilterSelect',
  inheritAttrs: false,
  props: {
    id: { type: String, required: true },
    modelValue: { type: String, required: true },
    options: { type: Array as PropType<{ value: string; label: string }[]>, required: true },
    allLabel: { type: String, required: true },
  },
  emits: ['update:modelValue'],
  setup(props, { emit, attrs }) {
    return () =>
      h(
        'select',
        {
          ...attrs,
          id: props.id,
          value: props.modelValue,
          onChange: (e: Event) => emit('update:modelValue', (e.target as HTMLSelectElement).value),
        },
        [
          h('option', { value: '' }, props.allLabel),
          ...props.options.map((o) => h('option', { value: o.value }, o.label)),
        ],
      )
  },
})

let wrapper: VueWrapper | null = null
beforeEach(() => {
  fetchMock.mockReset()
  fetchMock.mockImplementation(async () =>
    jsonResponse(200, { campaigns: [row], total: 1, page: 1, limit: 10 }),
  )
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
  vi.unstubAllGlobals()
})

async function mountList(query = '', stub = false) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/campaigns', component: CampaignListView }],
  })
  await router.push(`/campaigns${query}`)
  await router.isReady()
  wrapper = mount(CampaignListView, {
    attachTo: document.body,
    global: {
      plugins: [router],
      stubs: { RouterLink: RouterLinkStub, ...(stub ? { FilterSelect: FilterSelectStub } : {}) },
    },
  })
  await flushPromises()
  return wrapper
}

const requests = () => fetchMock.mock.calls.map(([u]) => String(u))

describe('Kampanyalar: durum süzgeci', () => {
  it('yerel <select> değil, shadcn Select tetikleyicisidir ve "Durum" etiketiyle adlanır', async () => {
    const w = await mountList()

    expect(w.find('select').exists()).toBe(false)
    const trigger = w.get('#campaign-filter')
    expect(trigger.attributes('data-slot')).toBe('select-trigger')
    expect(trigger.attributes('role')).toBe('combobox')
    expect((trigger.element as HTMLButtonElement).labels?.[0]?.textContent).toBe('Durum')
    expect(trigger.text()).toContain('Tüm durumlar')
  })

  it('adresteki ?status= değeriyle açılır: o durum seçili görünür ve istekte kullanılır', async () => {
    const w = await mountList('?status=completed')

    expect(requests()[0]).toBe('/api/campaigns?page=1&limit=10&status=completed')
    expect(w.get('#campaign-filter').text()).toContain('Tamamlandı')
    expect(w.get('#campaign-filter').attributes('data-active')).toBe('true')
  })

  it('geçersiz ?status= değeri yok sayılır (tüm durumlar)', async () => {
    await mountList('?status=bogus')

    expect(requests()[0]).toBe('/api/campaigns?page=1&limit=10&status=')
  })

  it('seçenekler Taslak, Onaylandı kuyrukta, Gönderiliyor ve Tamamlandı', async () => {
    const w = await mountList('', true)

    const options = w.findAll('select option').map((o) => o.text())
    expect(options).toEqual([
      'Tüm durumlar',
      'Taslak',
      'Onaylandı, kuyrukta',
      'Gönderiliyor',
      'Tamamlandı',
    ])
  })

  it('durum seçilince ilk sayfa o durumla istenir; "Tüm durumlar"a dönünce süzgeç kalkar', async () => {
    const w = await mountList('', true)

    await w.get('#campaign-filter').setValue('draft')
    await flushPromises()
    expect(requests().slice(-1)).toEqual(['/api/campaigns?page=1&limit=10&status=draft'])

    await w.get('#campaign-filter').setValue('')
    await flushPromises()
    expect(requests().slice(-1)).toEqual(['/api/campaigns?page=1&limit=10&status='])
  })

  it('satırlar kampanyaya açılır ve "Yeni kampanya" bağlantısı vardır', async () => {
    const w = await mountList()

    expect(w.findAll('[data-testid="campaign-page-row"]')).toHaveLength(1)
    expect(w.text()).toContain('Yeni kampanya')
  })
})
