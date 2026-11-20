<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { CirclePlusIcon, LayoutDashboardIcon, MegaphoneIcon, RefreshCwIcon } from '@lucide/vue'
import { getOverview } from '@/api/workspace'
import { useResource } from '@/composables/useResource'
import PageHeader from '@/components/PageHeader.vue'
import ErrorPanel from '@/components/ErrorPanel.vue'
import RecentCampaigns from '@/components/RecentCampaigns.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
const { data, loading, error, reload } = useResource(getOverview)
onMounted(reload)
const cards = computed(() =>
  data.value
    ? [
        {
          label: 'Kampanyalar',
          value: data.value.campaigns,
          to: '/campaigns',
          hint: `${data.value.draft} taslak · ${data.value.sending + data.value.approved} işlemi süren`,
        },
        {
          label: 'Kitledeki kişiler',
          value: data.value.users,
          to: '/audiences',
          hint: 'Sahte demo kullanıcıları',
        },
        {
          label: 'Gönderilen mesajlar',
          value: data.value.sent,
          to: '/campaigns?status=completed',
          hint: 'Sağlayıcı kabulü doğrulandı',
        },
        {
          label: 'Tamamlanmamış mesajlar',
          value: data.value.pending,
          to: '/campaigns',
          hint: 'Kuyrukta veya işleniyor',
        },
      ]
    : [],
)
</script>
<template>
  <main class="grid gap-8">
    <PageHeader
      :icon="LayoutDashboardIcon"
      title="Genel bakış"
      description="Kampanyalarınızın ve gönderimlerinizin güncel özeti."
    />
    <div class="flex flex-wrap gap-3">
      <Button as-child>
        <RouterLink to="/">
          <CirclePlusIcon aria-hidden="true" />
          Yeni kampanya
        </RouterLink>
      </Button>
      <Button as-child variant="outline">
        <RouterLink to="/campaigns">
          <MegaphoneIcon aria-hidden="true" />
          Kampanyalar
        </RouterLink>
      </Button>
      <Button variant="outline" :disabled="loading" @click="reload">
        <RefreshCwIcon
          aria-hidden="true"
          :class="{ 'animate-spin motion-reduce:animate-none': loading }"
        />
        Yenile
      </Button>
    </div>
    <Separator />
    <div v-if="loading" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4" role="status">
      <span class="sr-only">Genel bakış yükleniyor…</span
      ><Skeleton v-for="n in 4" :key="n" class="h-32" />
    </div>
    <ErrorPanel v-else-if="error" :title="error" :reasons="[]" :retryable="true" @retry="reload" />
    <template v-else-if="data">
      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card v-for="item in cards" :key="item.label" class="card-3d"
          ><CardHeader
            ><CardTitle
              ><h2 class="text-sm font-medium">{{ item.label }}</h2></CardTitle
            ></CardHeader
          ><CardContent class="grid gap-2"
            ><p class="text-3xl font-semibold tabular-nums">
              {{ item.value.toLocaleString('tr-TR') }}
            </p>
            <p class="text-muted-foreground text-xs">{{ item.hint }}</p>
            <RouterLink :to="item.to" class="text-primary text-sm underline underline-offset-4"
              >İncele</RouterLink
            ></CardContent
          ></Card
        >
      </div>
      <Separator />
      <div class="grid gap-4 sm:grid-cols-3">
        <Card class="card-3d"
          ><CardContent class="grid gap-1"
            ><h2 class="text-sm font-medium">Başarısız mesajlar</h2>
            <p class="text-2xl font-semibold">{{ data.failed }}</p>
            <p class="text-muted-foreground text-xs">
              Ayrıntılar ilgili kampanyanın gönderim takibinde.
            </p></CardContent
          ></Card
        >
        <Card class="card-3d"
          ><CardContent class="grid gap-1"
            ><h2 class="text-sm font-medium">Yeniden denemeyle tamamlanan</h2>
            <p class="text-2xl font-semibold">{{ data.recovered }}</p>
            <p class="text-muted-foreground text-xs">
              Birden fazla işleme denemesi sonrası gönderildi.
            </p></CardContent
          ></Card
        >
        <Card class="card-3d"
          ><CardContent class="grid gap-1"
            ><h2 class="text-sm font-medium">İçerik şablonları</h2>
            <p class="text-2xl font-semibold">{{ data.templates }}</p>
            <RouterLink to="/templates" class="text-primary text-sm underline underline-offset-4"
              >Şablonları aç</RouterLink
            ></CardContent
          ></Card
        >
      </div>
      <Separator />
      <Card class="card-3d"
        ><CardContent><RecentCampaigns /></CardContent
      ></Card>
      <p class="text-muted-foreground text-xs">
        Anlık veritabanı özeti; tarihsel grafik veya tüm mesajların okunduğu bilgisi değildir.
        Güncellemek için yenileyin.
      </p>
    </template>
  </main>
</template>
