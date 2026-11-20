<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import AppSidebar from '@/components/AppSidebar.vue'
import { Toaster } from '@/components/ui/sonner'
import { SidebarProvider, SidebarTrigger } from '@/components/ui/sidebar'
</script>

<template>
  <SidebarProvider>
    <AppSidebar />

    <!-- İçerik alanı. Ekranların kendi <main> öğesi vardır; bu yüzden SidebarInset (main) değil
         düz bir kap kullanılır. `min-w-0`: flex çocuğu uzun içerikle kenar çubuğunun yanından taşmasın. -->
    <div class="flex min-w-0 flex-1 flex-col">
      <!-- Üst başlık: kenar çubuğu düğmesi ve genel bakışa dönen marka.
           Sayfa başlığı (h1) her ekranda tek kalır; marka bağlantı olarak durur. -->
      <header
        class="bg-background/85 sticky top-0 z-20 flex h-14 items-center gap-2 border-b px-3 backdrop-blur sm:px-4"
        data-testid="app-header"
      >
        <!-- Dokunmatik ekranda 40 px'lik hedef; geniş ekranda shadcn'ın kompakt boyutu. -->
        <SidebarTrigger class="size-10 md:size-8" />
        <RouterLink
          to="/overview"
          class="text-primary hover:bg-accent focus-visible:ring-ring/50 -ml-1 flex items-center gap-2 rounded-lg px-2 py-2 outline-none focus-visible:ring-3"
          data-testid="brand-link"
        >
          <span class="text-base font-semibold tracking-tight sm:text-lg"
            >Insider Campaign Tool</span
          >
        </RouterLink>
      </header>

      <!-- Ortak sayfa kabuğu: aynı kenar boşlukları. Dar ekranlar (önizleme, gönderim takibi)
           kendi içlerinde max-w-3xl ile daralır; kampanya ekranı iki sütun için tam genişliği
           kullanır. -->
      <div class="mx-auto w-full max-w-6xl px-3 pt-6 pb-12 sm:px-4 sm:pt-8">
        <RouterView />
      </div>
    </div>

    <!-- Eylem sonuçları (başarı, uyarı, hata, bilgi): ekranın üstünde, 12 sn kalan bildirimler.
         Kartları `notify` (src/notify) gösterir. -->
    <Toaster position="top-center" expand :visible-toasts="4" :gap="22" :offset="16" />
  </SidebarProvider>
</template>
