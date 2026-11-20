<script setup lang="ts">
import { watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import {
  CalendarClockIcon,
  CirclePlusIcon,
  LayoutDashboardIcon,
  LayoutTemplateIcon,
  MegaphoneIcon,
  UsersRoundIcon,
} from '@lucide/vue'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarSeparator,
  useSidebar,
} from '@/components/ui/sidebar'
import { Separator } from '@/components/ui/separator'

// Navigation contains stable destinations, not a growing list of individual campaigns.
const route = useRoute()
const { setOpenMobile } = useSidebar()

const links = [
  { to: '/overview', label: 'Genel bakış', icon: LayoutDashboardIcon },
  { to: '/', label: 'Yeni kampanya', icon: CirclePlusIcon },
  { to: '/campaigns', label: 'Kampanyalar', icon: MegaphoneIcon },
  { to: '/scheduled', label: 'Zamanlanmış', icon: CalendarClockIcon },
  { to: '/audiences', label: 'Kitleler', icon: UsersRoundIcon },
  { to: '/templates', label: 'İçerik şablonları', icon: LayoutTemplateIcon },
]
function active(path: string) {
  return path === '/'
    ? route.path === '/'
    : route.path === path || route.path.startsWith(path + '/')
}
watch(
  () => route.fullPath,
  () => setOpenMobile(false),
)

// Sade ve düz: degrade, parıltı veya ikon karosu yok. Öğeler ince ayraçlarla ayrılır; etkin sayfa
// yumuşak bir zemin, solda ince vurgu çubuğu ve vurgu renginde ikonla belirtilir. Geçişler 200 ms,
// hareketi azaltma tercihinde kapanır.
const linkBase =
  'group/nav relative flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm outline-none ' +
  'transition-colors duration-200 ease-out focus-visible:ring-2 focus-visible:ring-sidebar-ring ' +
  'motion-reduce:transition-none'
const linkIdle =
  'text-sidebar-foreground/80 hover:bg-sidebar-accent/60 hover:text-sidebar-foreground'
const linkActive =
  'bg-sidebar-accent font-medium text-sidebar-accent-foreground ' +
  'before:absolute before:top-1/2 before:left-0 before:h-5 before:w-[3px] before:-translate-y-1/2 before:rounded-full before:bg-primary'
const iconBase = 'size-[18px] shrink-0 transition-colors duration-200 motion-reduce:transition-none'
const iconIdle = 'text-sidebar-foreground/55 group-hover/nav:text-sidebar-foreground'
const iconActive = 'text-primary'
</script>

<template>
  <Sidebar aria-label="Ana menü">
    <!-- Yükseklik uygulama üst başlığıyla (h-14) aynıdır: iki alt çizgi aynı hizada durur. -->
    <SidebarHeader class="h-14 justify-center px-4 py-0">
      <div class="flex items-center gap-3" data-testid="sidebar-brand">
        <span
          class="bg-foreground text-background grid size-8 shrink-0 place-items-center rounded-lg text-[11px] font-bold tracking-wider"
          aria-hidden="true"
          >IC</span
        >
        <div class="grid leading-tight">
          <span class="text-sm font-semibold tracking-tight">Çalışma alanı</span>
          <span class="text-sidebar-foreground/60 text-xs">Kampanya yönetimi</span>
        </div>
      </div>
    </SidebarHeader>

    <SidebarSeparator class="mx-0" />

    <SidebarContent>
      <SidebarGroup class="px-3 py-4">
        <SidebarGroupLabel
          class="text-sidebar-foreground/55 mb-1 px-3 text-[11px] font-semibold tracking-wider uppercase"
        >
          Menü
        </SidebarGroupLabel>
        <nav aria-label="Sayfalar" class="flex flex-col">
          <template v-for="(link, index) in links" :key="link.to">
            <Separator v-if="index > 0" class="bg-sidebar-border/70 my-1" />
            <RouterLink
              :to="link.to"
              :data-testid="link.to === '/' ? 'sidebar-new-campaign' : undefined"
              :class="[linkBase, active(link.to) ? linkActive : linkIdle]"
              :aria-current="active(link.to) ? 'page' : undefined"
              :data-active="active(link.to) ? 'true' : undefined"
            >
              <component
                :is="link.icon"
                :class="[iconBase, active(link.to) ? iconActive : iconIdle]"
                :stroke-width="1.75"
                aria-hidden="true"
              />
              <span class="truncate">{{ link.label }}</span>
            </RouterLink>
          </template>
        </nav>
      </SidebarGroup>
    </SidebarContent>

    <SidebarSeparator class="mx-0" />

    <SidebarFooter class="px-4 py-4">
      <p class="text-sidebar-foreground/65 text-xs leading-relaxed">
        Demo ortamı: gönderimler sahte sağlayıcıyla yapılır.
      </p>
    </SidebarFooter>
  </Sidebar>
</template>
