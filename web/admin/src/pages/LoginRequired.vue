<script setup lang="ts">
import { TgPlaceholder } from '../ui'
</script>

<template>
  <div class="box">
    <TgPlaceholder emoji="🔒" title="Нужен вход" description="Отправь боту /login" />
  </div>
</template>

<style scoped>
.box { max-width: 360px; margin: 20vh auto; }
</style>
