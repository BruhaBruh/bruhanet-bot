<script setup lang="ts">
defineOptions({ inheritAttrs: false })
defineProps<{ modelValue: string; options: { value: string; label: string }[] }>()
defineEmits<{ 'update:modelValue': [string] }>()
</script>

<template>
  <label class="tg-field">
    <select v-bind="$attrs" :value="modelValue" @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)">
      <option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option>
    </select>
    <span aria-hidden="true">⌄</span>
  </label>
</template>
