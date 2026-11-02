<script setup lang="ts">
defineProps<{ count: number }>()
</script>

<template>
  <span v-if="count > 0" class="tg-badge">{{ count }}</span>
</template>
