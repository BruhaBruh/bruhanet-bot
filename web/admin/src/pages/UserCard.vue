<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { get } from '../api'
import { formatDate, formatRub } from '../format'
import type { UserDetail } from '../types'
import { TgAvatar, TgButton, TgCell, TgSection } from '../ui'
import PaymentsTable from './PaymentsTable.vue'

const route = useRoute()
const d = ref<UserDetail>()
const error = ref('')

onMounted(async () => {
  try {
    d.value = await get<UserDetail>(`/api/users/${route.params.id}`)
  } catch (e) {
    error.value = String(e).includes('404') ? 'Юзер не найден' : 'Не удалось загрузить'
  }
})
</script>

<template>
  <TgButton mode="plain" size="s" class="back" @click="$router.push('/users')">← Юзеры</TgButton>
  <p v-if="error" class="err" role="alert">{{ error }}</p>
  <template v-if="d">
    <div class="head">
      <TgAvatar :text="d.user.username || String(d.user.telegramId)" :size="48" />
      <div class="who">
        <h2>{{ d.user.username ? '@' + d.user.username : d.user.telegramId }}</h2>
        <div class="id">{{ d.user.telegramId }}</div>
      </div>
    </div>
    <TgSection header="Подписка">
      <TgCell
        title="Подписка до"
        :after="d.panel.error ? 'панель недоступна' : d.panel.expireAt ? formatDate(d.panel.expireAt) : '—'"
        :after-sub="!d.panel.error && d.panel.status ? d.panel.status : undefined"
      />
    </TgSection>
    <TgSection header="Статистика">
      <TgCell title="С нами с" :after="formatDate(d.user.createdAt)" />
      <TgCell title="Подтверждённых оплат" :after="`${d.stats.confirmed} на ${formatRub(d.stats.confirmedSum)}`" />
      <TgCell title="Неподтверждённых авансов" :after="String(d.stats.unpaidAdvances)" />
      <TgCell title="Первая оплата" :after="d.stats.firstConfirmedAt ? formatDate(d.stats.firstConfirmedAt) : '—'" />
      <TgCell title="Последняя оплата" :after="d.stats.lastConfirmedAt ? formatDate(d.stats.lastConfirmedAt) : '—'" />
    </TgSection>
    <PaymentsTable header="Платежи" :items="d.payments" />
  </template>
</template>

<style scoped>
.back { margin-bottom: 8px; padding-left: 0; justify-content: flex-start; }
.head { display: flex; align-items: center; gap: 16px; margin-bottom: 24px; }
h2 { margin: 0; font-size: 20px; }
.id { color: var(--tg-hint-text); font-size: 13px; }
.err { color: var(--tg-danger); }
</style>
