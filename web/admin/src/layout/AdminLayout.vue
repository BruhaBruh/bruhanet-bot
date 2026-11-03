<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { post } from '../api'
import { TgAvatar, TgButton, TgCell } from '../ui'

const route = useRoute()
const router = useRouter()
const items = [
  { to: '/broadcasts', emoji: '📣', label: 'Рассылки', sub: 'скоро', color: 1 },
  { to: '/promos', emoji: '🎟', label: 'Промокоды', sub: 'скоро', color: 2 },
  { to: '/users', emoji: '👤', label: 'Юзеры', sub: 'поиск и карточки', color: 3 },
  { to: '/payments', emoji: '💳', label: 'Платежи', sub: 'история и выручка', color: 4 },
  { to: '/texts', emoji: '✏️', label: 'Тексты', sub: 'сообщения бота', color: 5 },
  { to: '/settings', emoji: '⚙️', label: 'Настройки', sub: 'тарифы и реквизиты', color: 6 },
] as const

// На телефоне меню — горизонтальная полоса: подкручиваем активный пункт в зону видимости.
const nav = ref<HTMLElement | null>(null)
watch(() => route.path, async () => {
  await nextTick()
  nav.value?.querySelector('.tg-active')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
}, { immediate: true })

async function logout() {
  await post('/api/logout')
  await router.push('/login-required')
}
</script>

<template>
  <div class="layout">
    <nav ref="nav" aria-label="Разделы">
      <div class="brand">BruhaNet</div>
      <TgCell v-for="i in items" :key="i.to" :to="i.to" :title="i.label" :subtitle="i.sub" :active="route.path.startsWith(i.to)">
        <template #before><TgAvatar :emoji="i.emoji" :color="i.color" /></template>
      </TgCell>
      <TgButton class="logout" mode="plain" @click="logout">выйти</TgButton>
    </nav>
    <main><div class="col"><router-view /></div></main>
  </div>
</template>

<style scoped>
.layout { display: flex; min-height: 100vh; }
nav { width: 280px; flex-shrink: 0; box-sizing: border-box; display: flex; flex-direction: column; gap: 2px; padding: 12px 8px;
  background: var(--tg-bg); border-right: 1px solid var(--tg-sep); position: sticky; top: 0; height: 100vh; overflow-y: auto; }
.brand { font-weight: 600; font-size: 20px; padding: 4px 8px 12px; }
.logout { margin-top: auto; align-self: stretch; }
main { flex: 1; min-width: 0; padding: 24px 16px; }
.col { max-width: 720px; margin: 0 auto; }
@media (max-width: 700px) {
  .layout { flex-direction: column; }
  nav { width: auto; height: auto; position: static; flex-direction: row; align-items: center; gap: 4px; overflow-x: auto; overflow-y: hidden;
    padding: 8px; border-right: 0; border-bottom: 1px solid var(--tg-sep); }
  .brand { display: none; }
  nav :deep(.tg-cell) { flex: none; padding: 0 12px 0 8px; gap: 8px; }
  nav :deep(.tg-cell)::before { display: none; }
  nav :deep(.tg-cell-body) { padding: 8px 0; }
  nav :deep(.tg-sub) { display: none; }
  nav :deep(.tg-av) { --s: 28px !important; }
  .logout { margin-top: 0; margin-left: auto; flex: none; align-self: center; }
  main { padding: 16px 12px; }
}
</style>
