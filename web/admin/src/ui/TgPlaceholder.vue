<script setup lang="ts">
defineProps<{ title: string; description?: string; emoji?: string }>()
</script>

<template>
  <div class="tg-ph">
    <div v-if="emoji" class="tg-ph-emoji" aria-hidden="true">{{ emoji }}</div>
    <div>
      <h2 class="tg-ph-title">{{ title }}</h2>
      <p v-if="description" class="tg-ph-desc">{{ description }}</p>
    </div>
    <slot />
  </div>
</template>
