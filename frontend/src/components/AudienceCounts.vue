<script setup lang="ts">
import { computed } from 'vue'
import type { Audience } from '@/api/campaigns'

const props = defineProps<{ audience: Audience }>()

const nf = new Intl.NumberFormat('tr-TR')
const hasRecipients = computed(() => props.audience.sms > 0 || props.audience.email > 0)
</script>

<template>
  <section aria-labelledby="audience-title">
    <h2 id="audience-title">Kitle önizleme</h2>
    <dl class="counts">
      <div class="count">
        <dt>Toplam eşleşen kişi</dt>
        <dd data-testid="total">{{ nf.format(audience.total) }}</dd>
      </div>
      <div class="count">
        <dt>SMS'e uygun kişi</dt>
        <dd data-testid="sms">{{ nf.format(audience.sms) }}</dd>
      </div>
      <div class="count">
        <dt>E-postaya uygun kişi</dt>
        <dd data-testid="email">{{ nf.format(audience.email) }}</dd>
      </div>
    </dl>
    <p class="note">
      Kanal sayıları yalnızca ilgili kanal için açıkça izin vermiş ve o kanalın adresi (telefon veya
      e-posta) bulunan kişileri içerir. İki kanala da uygun kişi ikisinde de sayılır; bu yüzden
      kanal sayılarının toplamı toplam kişi sayısı değildir.
    </p>
    <p v-if="audience.total === 0" class="warning" role="status">
      Bu tanımla hiç kullanıcı eşleşmedi.
    </p>
    <p v-else-if="!hasRecipients" class="warning" role="status">
      Eşleşen kişiler var ancak hiçbir kanalda izinli alıcı yok. Bu kampanya gönderime onaylanamaz.
    </p>
  </section>
</template>

<style scoped>
h2 {
  margin: 0 0 0.75rem;
  font-size: 1.1rem;
  font-weight: 700;
}
.counts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
  gap: 0.75rem;
  margin: 0;
}
.count {
  padding: 1rem;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: 8px;
}
dt {
  color: var(--app-muted);
  font-size: 0.9rem;
}
dd {
  margin: 0.25rem 0 0;
  font-size: 2rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.note {
  margin: 1em 0;
  color: var(--app-muted);
  font-size: 0.85rem;
}
.warning {
  margin: 1em 0;
  padding: 0.6rem 0.8rem;
  background: var(--app-warning-bg);
  border-radius: 8px;
  font-size: 0.9rem;
}
</style>
