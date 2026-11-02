<script setup lang="ts">
import { useRouter } from 'vue-router'
import { post } from '../api'

const router = useRouter()
const items = [
  { to: '/broadcasts', label: '📣 Рассылки' },
  { to: '/promos', label: '🎟 Промокоды' },
  { to: '/users', label: '👤 Юзеры' },
  { to: '/payments', label: '💳 Платежи' },
  { to: '/texts', label: '✏️ Тексты' },
  { to: '/settings', label: '⚙️ Настройки' },
]

async function logout() {
  await post('/api/logout')
  await router.push('/login-required')
}
</script>

<template>
  <div class="layout">
    <nav>
      <router-link v-for="i in items" :key="i.to" :to="i.to">{{ i.label }}</router-link>
      <button class="logout" @click="logout">выйти</button>
    </nav>
    <main><router-view /></main>
  </div>
</template>

<style scoped>
.layout { display: flex; min-height: 100vh; }
nav { width: 200px; flex-shrink: 0; display: flex; flex-direction: column; gap: 4px; padding: 16px 8px; background: #fff; border-right: 1px solid #d8dee4; }
nav a { padding: 8px 12px; border-radius: 6px; color: inherit; text-decoration: none; }
nav a.router-link-active { background: #ddf4ff; font-weight: 600; }
.logout { margin-top: auto; cursor: pointer; }
main { flex: 1; padding: 24px; min-width: 0; }
</style>
