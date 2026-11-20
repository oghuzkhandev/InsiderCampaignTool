<script setup lang="ts">
import { computed, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { PlusIcon } from '@lucide/vue'
import { campaignListVersion } from '@/campaign/recent'
import RecentCampaigns from '@/components/RecentCampaigns.vue'
import { Button } from '@/components/ui/button'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  useSidebar,
} from '@/components/ui/sidebar'

// Uygulama kenar çubuğu: yeni kampanya başlatma ve "Son kampanyalar" (CMP-001). Liste her
// ekranda bulunur; sayfa değişince veya bir ekran kampanyayı değiştirince (oluşturma, onay)
// yeniden okunur. Dar ekranda çekmece olarak açılır ve bir bağlantıya gidilince kapanır.
const route = useRoute()
const { setOpenMobile } = useSidebar()

const reloadKey = computed(() => `${route.fullPath}#${campaignListVersion.value}`)
watch(
  () => route.fullPath,
  () => setOpenMobile(false),
)
</script>

<template>
  <Sidebar aria-label="Kampanyalar">
    <SidebarHeader class="p-3">
      <Button as-child size="lg" class="w-full justify-start">
        <RouterLink to="/" data-testid="sidebar-new-campaign">
          <PlusIcon aria-hidden="true" />
          Yeni kampanya
        </RouterLink>
      </Button>
    </SidebarHeader>

    <SidebarContent>
      <SidebarGroup>
        <RecentCampaigns :reload-key="reloadKey" />
      </SidebarGroup>
    </SidebarContent>

    <SidebarFooter class="text-sidebar-foreground/70 p-3 text-xs">
      Demo ortamı: gönderimler sahte sağlayıcıyla yapılır.
    </SidebarFooter>
  </Sidebar>
</template>
