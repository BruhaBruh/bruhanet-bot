<script setup lang="ts">
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })
defineProps<{ modelValue: string; options: { value: string; label: string }[] }>()
defineEmits<{ 'update:modelValue': [string] }>()

// class/style stay on the root label, the rest goes to the native control
const attrs = useAttrs()
const rest = computed(() => {
  const { class: _c, style: _s, ...r } = attrs
  return r
})
</script>

<template>
  <label class="tg-field" :class="attrs.class" :style="attrs.style as any">
    <select v-bind="rest" :value="modelValue" @change="$emit('update:modelValue', ($event.target as HTMLSelectElement).value)">
      <option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option>
    </select>
    <span aria-hidden="true">⌄</span>
  </label>
</template>
