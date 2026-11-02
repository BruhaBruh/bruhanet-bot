<script setup lang="ts">
defineProps<{ modelValue: string; placeholder?: string; type?: string; icon?: 'search' }>()
defineEmits<{ 'update:modelValue': [string] }>()
</script>

<template>
  <label class="tg-field">
    <span v-if="icon === 'search'" aria-hidden="true">🔍</span>
    <input
      :value="modelValue"
      :placeholder="placeholder"
      :type="type ?? (icon === 'search' ? 'search' : 'text')"
      @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
  </label>
</template>
