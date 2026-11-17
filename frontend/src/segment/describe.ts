import type { Condition, Segment } from '@/api/campaigns'

const nf = new Intl.NumberFormat('tr-TR')

function formatValue(value: Condition['value']): string {
  if (Array.isArray(value)) return value.join(' veya ')
  return typeof value === 'number' ? nf.format(value) : value
}

function describeCondition(c: Condition): string {
  const value = formatValue(c.value)

  switch (`${c.field} ${c.op}`) {
    case 'city eq':
    case 'city in':
      return `Şehir: ${value}`
    case 'last_cart_add_at within_last_days':
      return `Son ${value} günde sepete ürün eklemiş`
    case 'last_cart_add_at not_within_last_days':
      return `Son ${value} günde sepete ürün eklememiş`
    case 'last_purchase_at within_last_days':
      return `Son ${value} günde satın alma yapmış`
    case 'last_purchase_at not_within_last_days':
      return `Son ${value} günde satın alma yapmamış`
    case 'order_count eq':
      return `Sipariş sayısı: ${value}`
    case 'order_count gte':
      return `Sipariş sayısı en az ${value}`
    case 'order_count lte':
      return `Sipariş sayısı en fazla ${value}`
    default:
      return `${c.field} ${c.op} ${value}`
  }
}

/**
 * Doğrulanmış segment yapısından okunur koşul listesi üretir. Model metni
 * değil, backend'in doğruladığı yapı kullanılır (SEG-001).
 */
export function describeSegment(segment: Segment): string[] {
  return segment.all.map(describeCondition)
}
