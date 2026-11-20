<script setup lang="ts">
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
// uyan kişiler" listesi hem Kitleler sayfası aynı görünümü kullanır. Eksik ad, e-posta ve telefon
// açıkça "yok" yazılır. Veri HTML olarak yorumlanmaz.
defineProps<{
  members: AudienceMember[]
  /** Ekran okuyucuya okunan tablo açıklaması (görünmez). */
  caption: string
}>()

// Telefon ve tablet genişliğinde (xl altı) tablo, satır başına bir karta dönüşür: sayfa kapsayıcısı
// 768 px ile sınırlı olduğundan altı sütun yan yana ancak geniş ekranda rahat sığar. Başlık satırı
// ekran okuyucuya kalır, kart içinde her değerin etiketi görünür.
const rowClass =
  'max-xl:grid max-xl:gap-1.5 max-xl:rounded-lg max-xl:border max-xl:p-3 max-xl:last:border-b'
const cellClass =
  'whitespace-normal wrap-anywhere max-xl:flex max-xl:items-baseline max-xl:gap-2 max-xl:p-0'
const labelClass = 'text-muted-foreground w-28 shrink-0 text-xs xl:hidden'
</script>

<template>
  <Table class="max-xl:block">
    <TableCaption class="sr-only">{{ caption }}</TableCaption>
    <TableHeader class="max-xl:sr-only">
      <TableRow>
        <TableHead>Ad</TableHead>
        <TableHead>Şehir</TableHead>
        <TableHead>E-posta</TableHead>
        <TableHead>Telefon</TableHead>
        <TableHead>SMS uygunluğu</TableHead>
        <TableHead>E-posta uygunluğu</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody class="max-xl:grid max-xl:gap-3">
      <TableRow
        v-for="member in members"
        :key="member.id"
        :class="rowClass"
        data-testid="member-row"
      >
        <TableCell :class="cellClass" data-testid="member-name">
          <span :class="labelClass">Ad</span>
          <span v-if="member.first_name" class="font-medium">{{ member.first_name }}</span>
          <span v-else class="text-muted-foreground italic" data-missing="true">
            {{ MISSING.name }}
          </span>
        </TableCell>
        <TableCell :class="cellClass" data-testid="member-city">
          <span :class="labelClass">Şehir</span>
          <span>{{ member.city }}</span>
        </TableCell>
        <TableCell :class="cellClass" data-testid="member-email">
          <span :class="labelClass">E-posta</span>
          <span v-if="member.email">{{ member.email }}</span>
          <span v-else class="text-muted-foreground italic" data-missing="true">
            {{ MISSING.email }}
          </span>
        </TableCell>
        <TableCell :class="cellClass" data-testid="member-phone">
          <span :class="labelClass">Telefon</span>
          <span v-if="member.phone">{{ member.phone }}</span>
          <span v-else class="text-muted-foreground italic" data-missing="true">
            {{ MISSING.phone }}
          </span>
        </TableCell>
        <TableCell :class="cellClass" data-testid="member-sms">
          <span :class="labelClass">SMS uygunluğu</span>
          <ChannelStatusBadge channel="sms" :status="member.sms_status" />
        </TableCell>
        <TableCell :class="cellClass" data-testid="member-email-status">
          <span :class="labelClass">E-posta uygunluğu</span>
          <ChannelStatusBadge channel="email" :status="member.email_status" />
        </TableCell>
      </TableRow>
    </TableBody>
  </Table>
</template>
