<script setup lang="ts">
import {
  MailCheckIcon,
  MailIcon,
  MapPinIcon,
  MessageSquareIcon,
  PhoneIcon,
  UserRoundIcon,
} from '@lucide/vue'
import type { AudienceMember } from '@/api/audience'
import { MISSING } from '@/audience/members'
import ChannelStatusBadge from '@/components/ChannelStatusBadge.vue'
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

// Kişi satırları (ad, şehir, e-posta, telefon, SMS ve e-posta uygunluğu): hem kampanyanın "Segmente
// uyan kişiler" listesi hem Kitleler sayfası aynı görünümü kullanır. Geniş kapsayıcıda çizgili bir
// tablodur (dış çerçeve, satır ve sütun çizgileri, ikonlu başlıklar); dar kapsayıcıda (< 56 rem)
// her kişi çizgilerle bölünmüş bir karttır. Geçiş EKRAN değil KAPSAYICI genişliğine bağlıdır
// (container query): sayfa ne kadar dar bir sütunda olursa olsun sütunlar ezilip kırılmaz. Eksik ad,
// e-posta ve telefon açıkça "yok" yazılır. Veri HTML olarak yorumlanmaz.
defineProps<{
  members: AudienceMember[]
  /** Ekran okuyucuya okunan tablo açıklaması (görünmez). */
  caption: string
}>()

const columns = [
  { id: 'name', label: 'Ad', icon: UserRoundIcon },
  { id: 'city', label: 'Şehir', icon: MapPinIcon },
  { id: 'email', label: 'E-posta', icon: MailIcon },
  { id: 'phone', label: 'Telefon', icon: PhoneIcon },
  { id: 'sms', label: 'SMS uygunluğu', icon: MessageSquareIcon },
  { id: 'emailStatus', label: 'E-posta uygunluğu', icon: MailCheckIcon },
] as const

// Geniş: sütun çizgisi, rahat iç boşluk, tek satır. Dar: her hücre etiketli bir satır, aralarında çizgi.
const cellClass =
  'align-middle @4xl:border-r @4xl:px-3 @4xl:py-2.5 @4xl:last:border-r-0 ' +
  '@max-4xl:flex @max-4xl:items-center @max-4xl:gap-3 @max-4xl:border-b @max-4xl:px-3 @max-4xl:py-2 ' +
  '@max-4xl:whitespace-normal @max-4xl:last:border-b-0'
const rowClass =
  'last:border-b-0 @max-4xl:overflow-hidden @max-4xl:rounded-lg @max-4xl:border @max-4xl:last:border-b'
const labelClass =
  'text-muted-foreground flex w-36 shrink-0 items-center gap-1.5 text-xs @4xl:hidden'
const iconClass = 'text-muted-foreground size-4 shrink-0'
</script>

<template>
  <div class="@container">
    <div class="@4xl:overflow-hidden @4xl:rounded-xl @4xl:border">
      <Table class="@max-4xl:block">
        <TableCaption class="sr-only">{{ caption }}</TableCaption>
        <TableHeader class="bg-muted/50 @max-4xl:sr-only">
          <TableRow class="hover:bg-transparent">
            <TableHead
              v-for="column in columns"
              :key="column.id"
              class="h-11 px-3 text-xs font-semibold @4xl:border-r @4xl:last:border-r-0"
            >
              <span class="inline-flex items-center gap-2">
                <component :is="column.icon" :class="iconClass" aria-hidden="true" />
                {{ column.label }}
              </span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody class="@max-4xl:grid @max-4xl:gap-3">
          <TableRow
            v-for="member in members"
            :key="member.id"
            :class="rowClass"
            data-testid="member-row"
          >
            <TableCell :class="cellClass" data-testid="member-name">
              <span :class="labelClass">
                <UserRoundIcon class="size-3.5" aria-hidden="true" />Ad
              </span>
              <span v-if="member.first_name" class="font-medium">{{ member.first_name }}</span>
              <span v-else class="text-muted-foreground italic" data-missing="true">
                {{ MISSING.name }}
              </span>
            </TableCell>
            <TableCell :class="cellClass" data-testid="member-city">
              <span :class="labelClass">
                <MapPinIcon class="size-3.5" aria-hidden="true" />Şehir
              </span>
              <span>{{ member.city }}</span>
            </TableCell>
            <TableCell
              :class="[cellClass, 'wrap-anywhere @4xl:min-w-56 @4xl:whitespace-normal']"
              data-testid="member-email"
            >
              <span :class="labelClass">
                <MailIcon class="size-3.5" aria-hidden="true" />E-posta
              </span>
              <span v-if="member.email">{{ member.email }}</span>
              <span v-else class="text-muted-foreground italic" data-missing="true">
                {{ MISSING.email }}
              </span>
            </TableCell>
            <TableCell :class="cellClass" data-testid="member-phone">
              <span :class="labelClass">
                <PhoneIcon class="size-3.5" aria-hidden="true" />Telefon
              </span>
              <span v-if="member.phone" class="tabular-nums">{{ member.phone }}</span>
              <span v-else class="text-muted-foreground italic" data-missing="true">
                {{ MISSING.phone }}
              </span>
            </TableCell>
            <TableCell :class="cellClass" data-testid="member-sms">
              <span :class="labelClass">
                <MessageSquareIcon class="size-3.5" aria-hidden="true" />SMS uygunluğu
              </span>
              <ChannelStatusBadge channel="sms" :status="member.sms_status" />
            </TableCell>
            <TableCell :class="cellClass" data-testid="member-email-status">
              <span :class="labelClass">
                <MailCheckIcon class="size-3.5" aria-hidden="true" />E-posta uygunluğu
              </span>
              <ChannelStatusBadge channel="email" :status="member.email_status" />
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
