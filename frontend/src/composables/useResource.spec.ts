import { describe, expect, it } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useResource } from './useResource'

describe('useResource istek yaşam döngüsü', () => {
  it('iptal edilen eski yanıt yeni sonucu ezemez ve ekran kapanınca istek iptal edilir', async () => {
    const requests: { signal: AbortSignal; resolve: (value: string) => void }[] = []
    const wrapper = mount(
      defineComponent({
        setup() {
          const resource = useResource(
            (signal) => new Promise<string>((resolve) => requests.push({ signal, resolve })),
          )
          return () =>
            h('div', [
              h('button', { onClick: resource.reload }, 'Yenile'),
              h('p', resource.data.value ?? ''),
            ])
        },
      }),
    )
    await wrapper.get('button').trigger('click')
    await wrapper.get('button').trigger('click')
    const first = requests[0],
      second = requests[1]
    if (!first || !second) throw new Error('İki istek bekleniyordu')
    expect(first.signal.aborted).toBe(true)
    second.resolve('Yeni sonuç')
    await flushPromises()
    first.resolve('Eski sonuç')
    await flushPromises()
    expect(wrapper.get('p').text()).toBe('Yeni sonuç')
    await wrapper.get('button').trigger('click')
    const last = requests[2]
    if (!last) throw new Error('Son istek bekleniyordu')
    wrapper.unmount()
    expect(last.signal.aborted).toBe(true)
  })
})
