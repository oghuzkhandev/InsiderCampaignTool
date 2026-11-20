import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { UNSAVED_MESSAGE, useUnsavedChangesGuard } from '../useUnsavedChangesGuard'

// `beforeunload` dinleyicisi pencerededir: bileşenler kapatılmazsa testler birbirine sızar.
let mounted: VueWrapper[] = []
afterEach(() => {
  mounted.forEach((w) => w.unmount())
  mounted = []
})

function setup(initialDirty: boolean, confirmLeave: (message: string) => boolean) {
  const dirty = ref(initialDirty)
  const Editor = defineComponent({
    setup() {
      useUnsavedChangesGuard(() => dirty.value, confirmLeave)
      return () => h('div', 'editor')
    },
  })
  const Other = defineComponent({ render: () => h('div', 'other') })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: Other },
      { path: '/campaigns/:id', component: Editor },
    ],
  })
  const App = defineComponent({ render: () => h(RouterView) })
  return { dirty, router, App }
}

async function mountAt(path: string, initialDirty: boolean, confirmLeave: (m: string) => boolean) {
  const ctx = setup(initialDirty, confirmLeave)
  await ctx.router.push(path)
  const wrapper = mount(ctx.App, { global: { plugins: [ctx.router] } })
  mounted.push(wrapper)
  await flushPromises()
  return { ...ctx, wrapper }
}

describe('useUnsavedChangesGuard', () => {
  describe('uygulama içi gezinme', () => {
    it('değişiklik yoksa sormadan gezinir', async () => {
      const confirmLeave = vi.fn<(message: string) => boolean>(() => false)
      const { router } = await mountAt('/campaigns/a', false, confirmLeave)

      await router.push('/')

      expect(confirmLeave).not.toHaveBeenCalled()
      expect(router.currentRoute.value.path).toBe('/')
    })

    it('değişiklik varken uyarır ve kullanıcı vazgeçerse sayfada kalır', async () => {
      const confirmLeave = vi.fn<(message: string) => boolean>(() => false)
      const { router, wrapper } = await mountAt('/campaigns/a', true, confirmLeave)

      await router.push('/')
      await flushPromises()

      expect(confirmLeave).toHaveBeenCalledWith(UNSAVED_MESSAGE)
      expect(router.currentRoute.value.path).toBe('/campaigns/a')
      expect(wrapper.text()).toBe('editor')
    })

    it('değişiklik varken kullanıcı onaylarsa gezinir', async () => {
      const confirmLeave = vi.fn<(message: string) => boolean>(() => true)
      const { router } = await mountAt('/campaigns/a', true, confirmLeave)

      await router.push('/')

      expect(confirmLeave).toHaveBeenCalledTimes(1)
      expect(router.currentRoute.value.path).toBe('/')
    })

    it('başka kampanyaya geçişte (aynı bileşen) de uyarır', async () => {
      const confirmLeave = vi.fn<(message: string) => boolean>(() => false)
      const { router } = await mountAt('/campaigns/a', true, confirmLeave)

      await router.push('/campaigns/b')

      expect(confirmLeave).toHaveBeenCalledTimes(1)
      expect(router.currentRoute.value.path).toBe('/campaigns/a')
    })

    it('değişiklik kaydedilince (artık kirli değil) uyarmaz', async () => {
      const confirmLeave = vi.fn<(message: string) => boolean>(() => false)
      const { router, dirty } = await mountAt('/campaigns/a', true, confirmLeave)

      dirty.value = false
      await router.push('/')

      expect(confirmLeave).not.toHaveBeenCalled()
      expect(router.currentRoute.value.path).toBe('/')
    })
  })

  describe('sekmeyi kapatma ve yenileme (beforeunload)', () => {
    const fire = () => {
      const event = new Event('beforeunload', { cancelable: true })
      window.dispatchEvent(event)
      return event
    }

    it('değişiklik varken tarayıcının uyarısını ister', async () => {
      await mountAt('/campaigns/a', true, () => true)
      expect(fire().defaultPrevented).toBe(true)
    })

    it('değişiklik yokken istemez', async () => {
      await mountAt('/campaigns/a', false, () => true)
      expect(fire().defaultPrevented).toBe(false)
    })

    it('bileşen kapanınca dinleyici kaldırılır (başka sayfada uyarı çıkmaz)', async () => {
      const { wrapper } = await mountAt('/campaigns/a', true, () => true)
      expect(fire().defaultPrevented).toBe(true)
      wrapper.unmount()
      expect(fire().defaultPrevented).toBe(false)
    })
  })

  it('yönlendirici olmayan ortamda hata vermez ve yalnızca tarayıcı uyarısını kurar', () => {
    const Host = defineComponent({
      setup() {
        useUnsavedChangesGuard(() => true)
        return () => h('div')
      },
    })
    mounted.push(mount(Host))
    const event = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
  })
})
