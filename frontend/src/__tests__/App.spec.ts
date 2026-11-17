import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import App from '../App.vue'
import router from '../router'

describe('App', () => {
  it('ana adreste kampanya önizleme ekranını gösterir', async () => {
    await router.push('/')
    await router.isReady()

    const wrapper = mount(App, { global: { plugins: [router] } })
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe('Kampanya önizleme')
    expect(wrapper.find('textarea#brief').exists()).toBe(true)
  })
})
