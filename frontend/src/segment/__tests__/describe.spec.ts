import { describe, expect, it } from 'vitest'
import { describeSegment } from '../describe'
import type { Condition } from '@/api/campaigns'

const lines = (...all: Condition[]) => describeSegment({ all })

describe('describeSegment', () => {
  it('örnek brief koşullarını okunur yazar', () => {
    expect(
      lines(
        { field: 'city', op: 'eq', value: 'İstanbul' },
        { field: 'last_cart_add_at', op: 'within_last_days', value: 30 },
        { field: 'last_purchase_at', op: 'not_within_last_days', value: 30 },
      ),
    ).toEqual([
      'Şehir: İstanbul',
      'Son 30 günde sepete ürün eklemiş',
      'Son 30 günde satın alma yapmamış',
    ])
  })

  it('şehir listesini veya ile birleştirir', () => {
    expect(lines({ field: 'city', op: 'in', value: ['İstanbul', 'Ankara'] })).toEqual([
      'Şehir: İstanbul veya Ankara',
    ])
  })

  it('diğer operatörleri yazar', () => {
    expect(
      lines(
        { field: 'last_cart_add_at', op: 'not_within_last_days', value: 7 },
        { field: 'last_purchase_at', op: 'within_last_days', value: 10 },
        { field: 'order_count', op: 'eq', value: 3 },
        { field: 'order_count', op: 'gte', value: 2 },
        { field: 'order_count', op: 'lte', value: 1000 },
      ),
    ).toEqual([
      'Son 7 günde sepete ürün eklememiş',
      'Son 10 günde satın alma yapmış',
      'Sipariş sayısı: 3',
      'Sipariş sayısı en az 2',
      'Sipariş sayısı en fazla 1.000',
    ])
  })

  it('tanınmayan koşulu ham biçimde gösterir, düşürmez', () => {
    expect(lines({ field: 'x', op: 'y', value: 1 })).toEqual(['x y 1'])
  })

  it('koşulsuz segment boş liste verir', () => {
    expect(lines()).toEqual([])
  })
})
