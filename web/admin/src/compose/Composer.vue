<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ApiError, get, post } from '../api'
import TgEditor from '../editor/TgEditor.vue'
import { sanitize, visibleLen, visibleText } from '../editor/serialize'
import type { Broadcast, DirectMessage } from '../types'
import { TgButton } from '../ui'
import PhotoPicker from './PhotoPicker.vue'

// broadcast — всем из панели; direct — одному юзеру (userId)
const props = defineProps<{ mode: 'broadcast' | 'direct'; userId?: number }>()
const emit = defineEmits<{ sent: [Broadcast | DirectMessage] }>()

const text = ref('')
const photo = ref<string>()
const busy = ref(false)
const error = ref('')
const testSent = ref(false)
const confirming = ref(false)
// null — грузится, undefined — панель недоступна
const recipients = ref<number | null | undefined>(null)
const uploading = ref(false)
// ключ идемпотентности — на конкретное содержимое: повтор без правок после сбоя
// не создаст вторую рассылку, а правка даёт новый ключ (иначе сервер вернёт старую)
let key = crypto.randomUUID()

async function loadRecipients() {
  recipients.value = null
  try {
    recipients.value = (await get<{ count: number }>('/api/broadcasts/recipients')).count
  } catch {
    recipients.value = undefined
  }
}
onMounted(() => props.mode === 'broadcast' && loadRecipients())

const len = computed(() => visibleLen(text.value))
const limit = computed(() => (photo.value ? 1024 : 4096))
const valid = computed(() => !uploading.value && len.value <= limit.value && (!!photo.value || visibleText(text.value).trim() !== ''))
const preview = computed(() => sanitize(text.value))

// правка черновика отменяет начатое подтверждение и меняет ключ
watch([text, photo], () => {
  key = crypto.randomUUID()
  confirming.value = false
  testSent.value = false
})

const body = () => ({ text: text.value, ...(photo.value ? { photo: photo.value } : {}) })

async function run(fn: () => Promise<void>) {
  error.value = ''
  testSent.value = false
  busy.value = true
  try {
    await fn()
  } catch (e) {
    error.value =
      e instanceof ApiError && e.body.error === 'panel_unavailable'
        ? 'Панель недоступна, рассылка не создана'
        : e instanceof ApiError && e.body.message
          ? e.body.message
          : 'Не удалось отправить, попробуйте ещё раз'
  } finally {
    busy.value = false
  }
}

const sendTest = () =>
  run(async () => {
    await post('/api/broadcasts/preview-send', body())
    testSent.value = true
  })

function reset() {
  text.value = ''
  photo.value = undefined
}

const broadcast = () =>
  run(async () => {
    confirming.value = false
    const { broadcast } = await post<{ broadcast: Broadcast }>('/api/broadcasts', { idempotencyKey: key, ...body() })
    reset()
    emit('sent', broadcast)
  })

const direct = () =>
  run(async () => {
    const b = body()
    const { id } = await post<{ id: number }>(`/api/users/${props.userId}/messages`, b)
    reset()
    emit('sent', { id, telegramId: props.userId!, ...b, status: 'pending', createdAt: new Date().toISOString() })
  })
</script>

<template>
  <div class="composer">
    <TgEditor v-model="text" :disabled="busy" />
    <div class="row">
      <PhotoPicker v-model="photo" :disabled="busy" @uploading="uploading = $event" />
      <span class="count" :class="{ over: len > limit }">{{ len }} / {{ limit }}</span>
    </div>

    <div class="label">Превью</div>
    <!-- только разрешённые теги после sanitize -->
    <div class="preview tg-msg">
      <div v-if="photo" class="ph">🖼 фото</div>
      <div v-html="preview" />
    </div>

    <p v-if="mode === 'broadcast' && recipients === undefined" class="tg-err" role="alert">
      Панель недоступна, количество получателей неизвестно
      <TgButton mode="plain" size="s" @click="loadRecipients">Повторить</TgButton>
    </p>
    <p v-if="error" class="tg-err" role="alert">{{ error }}</p>
    <p v-if="testSent" class="tg-ok" role="status">Отправлено в Telegram</p>

    <div class="actions">
      <template v-if="mode === 'broadcast'">
        <template v-if="confirming">
          <TgButton :loading="busy" :disabled="!valid" @click="broadcast">Точно разослать {{ recipients }} юзерам?</TgButton>
          <TgButton mode="plain" :disabled="busy" @click="confirming = false">Отмена</TgButton>
        </template>
        <TgButton v-else :loading="busy" :disabled="!valid || !recipients" @click="confirming = true">
          Разослать всем{{ recipients != null ? ` (${recipients})` : '' }}
        </TgButton>
      </template>
      <TgButton v-else :loading="busy" :disabled="!valid" @click="direct">Отправить</TgButton>
      <TgButton mode="bezeled" :disabled="busy || !valid" @click="sendTest">Отправить себе</TgButton>
    </div>
  </div>
</template>

<style scoped>
.row { display: flex; justify-content: space-between; align-items: flex-start; gap: 8px; margin-top: 8px; }
.count { font-size: 13px; color: var(--tg-hint-text); white-space: nowrap; }
.count.over { color: var(--tg-danger-text); }
.label { font-size: 13px; color: var(--tg-hint-text); margin: 16px 0 6px; }
.preview { min-height: 1.3em; }
.ph { color: var(--tg-hint-text); font-size: 13px; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 16px; }
</style>
