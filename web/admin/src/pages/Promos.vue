<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ApiError, get, post } from '../api'
import { formatDate } from '../format'
import { generateCode, promoBody, promoGives, promoUntil, promoUsage, validatePromo, type PromoForm } from '../promo'
import type { Promo, PromoActivation } from '../types'
import { TgButton, TgCell, TgInput, TgPlaceholder, TgSection, TgSelect, TgSpinner } from '../ui'

const status = ref('active')
const items = ref<Promo[]>()
const loadError = ref(false)

let loadSeq = 0
async function load() {
  const seq = ++loadSeq // устаревший ответ при быстром переключении фильтра не перетирает новый
  try {
    const r = (await get<{ items: Promo[] }>(`/api/promos?status=${status.value}`)).items
    if (seq !== loadSeq) return
    items.value = r
    loadError.value = false
  } catch {
    if (seq === loadSeq) loadError.value = true
  }
}
onMounted(load)

// раскрытая строка → её активации (undefined — грузятся, null — ошибка)
const open = ref<number | null>(null)
const acts = ref<PromoActivation[] | null>()
async function toggle(id: number) {
  if (open.value === id) {
    open.value = null
    return
  }
  open.value = id
  acts.value = undefined
  try {
    const r = (await get<{ items: PromoActivation[] }>(`/api/promos/${id}/activations`)).items
    if (open.value === id) acts.value = r
  } catch {
    if (open.value === id) acts.value = null
  }
}

const disabling = ref<number | null>(null)
const disableError = ref(false)
async function disable(p: Promo) {
  disabling.value = p.id
  disableError.value = false
  try {
    await post(`/api/promos/${p.id}/disable`)
    await load()
  } catch {
    disableError.value = true
  } finally {
    disabling.value = null
  }
}

const marks = (p: Promo) =>
  [promoUsage(p), p.validUntil && promoUntil(p.validUntil), p.onlyNew && 'только новым', p.telegramId && (p.username ? '@' + p.username : `ID ${p.telegramId}`)]
    .filter(Boolean)
    .join(' · ')

const empty = (): PromoForm => ({ code: '', kind: 'percent', value: '', maxActivations: '', validUntil: '', onlyNew: false, user: '' })
const form = reactive(empty())
const kinds = [
  { value: 'percent', label: 'скидка %' },
  { value: 'rub', label: 'скидка ₽' },
  { value: 'days', label: 'бонусные дни' },
] as const
const busy = ref(false)
const error = ref('')
const created = ref('')

async function create() {
  if (busy.value) return
  created.value = ''
  error.value = validatePromo(form)
  if (error.value) return
  busy.value = true
  try {
    const { promo } = await post<{ promo: Promo }>('/api/promos', promoBody(form))
    created.value = `Создан ${promo.code}`
    Object.assign(form, empty())
    await load()
  } catch (e) {
    error.value = e instanceof ApiError && e.body.message ? e.body.message : 'Не удалось создать, попробуйте ещё раз'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <h1 class="tg-visually-hidden">Промокоды</h1>
  <div class="grid">
    <div>
      <TgSelect
        v-model="status"
        class="filter"
        aria-label="Фильтр"
        :options="[{ value: 'active', label: 'Активные' }, { value: 'all', label: 'Все' }]"
        @change="load"
      />
      <p v-if="disableError" class="tg-err" role="alert">Не удалось выключить</p>
      <p v-if="loadError" class="tg-err" role="alert">Не удалось загрузить</p>
      <div v-else-if="!items" class="tg-loading" role="status"><TgSpinner /><span class="tg-visually-hidden">Загрузка…</span></div>
      <TgPlaceholder v-else-if="!items.length" emoji="🎟" title="Промокодов нет" />
      <TgSection v-else>
        <template v-for="p in items" :key="p.id">
          <TgCell
            class="row"
            role="button"
            tabindex="0"
            :aria-expanded="open === p.id"
            :title="p.code"
            :subtitle="`${promoGives(p)} · ${marks(p)}`"
            @click="toggle(p.id)"
            @keydown.enter.self="toggle(p.id)"
            @keydown.space.self.prevent="toggle(p.id)"
          >
            <template #after>
              <span v-if="p.disabled" class="tg-sub">выключен</span>
              <TgButton v-else mode="plain" size="s" :loading="disabling === p.id" @click.stop="disable(p)">Выключить</TgButton>
            </template>
          </TgCell>
          <div v-if="open === p.id" class="acts">
            <TgSpinner v-if="acts === undefined" />
            <p v-else-if="acts === null" class="tg-err">Не удалось загрузить</p>
            <p v-else-if="!acts.length" class="tg-sub">Ещё никто не активировал</p>
            <template v-else>
              <p v-for="a in acts" :key="a.id" class="tg-sub">
                {{ a.username ? '@' + a.username : a.telegramId }} · {{ formatDate(a.createdAt) }}{{ a.paymentId ? ` · платёж #${a.paymentId}` : '' }}
              </p>
            </template>
          </div>
        </template>
      </TgSection>
    </div>

    <form @submit.prevent="create">
      <TgSection header="Новый промокод" footer="Пустой код сгенерирует сервер.">
        <div class="code">
          <TgInput v-model="form.code" grouped placeholder="Код, например SUMMER25" />
          <TgButton mode="plain" aria-label="Сгенерировать код" @click="form.code = generateCode()">🎲</TgButton>
        </div>
        <fieldset class="kinds">
          <legend class="tg-visually-hidden">Тип</legend>
          <label v-for="k in kinds" :key="k.value"><input v-model="form.kind" type="radio" name="kind" :value="k.value" /> {{ k.label }}</label>
        </fieldset>
        <TgInput v-model="form.value" grouped type="number" min="1" placeholder="Значение" />
      </TgSection>
      <TgSection header="Ограничения (необязательно)">
        <div class="col">
          <TgInput v-model="form.maxActivations" grouped type="number" min="1" placeholder="Макс. активаций" />
          <TgInput v-model="form.validUntil" grouped type="date" aria-label="Действует до (включительно)" />
          <label class="check"><input v-model="form.onlyNew" type="checkbox" /> только новым (ни одной оплаты)</label>
          <TgInput v-model="form.user" grouped placeholder="Только для юзера: @username или ID" />
        </div>
      </TgSection>
      <p v-if="error" class="tg-err" role="alert">{{ error }}</p>
      <p v-if="created" class="tg-ok" role="status">{{ created }}</p>
      <TgButton :loading="busy" @click="create">Создать</TgButton>
    </form>
  </div>
</template>

<style scoped>
/* одна колонка: контент не шире 720px, две колонки там тесные; minmax(0) — длинные строки не распирают страницу */
.grid { display: grid; grid-template-columns: minmax(0, 1fr); }
.filter { margin-bottom: 16px; }
.row { cursor: pointer; }
/* подпись — вся информация о коде: переносим, не обрезаем */
.row :deep(.tg-sub) { white-space: normal; }
.acts { padding: 4px 16px 12px; }
.acts p { margin: 4px 0; white-space: normal; }
.code { display: flex; gap: 1px; background: var(--tg-sep); }
.code > :deep(.tg-field) { flex: 1; min-width: 0; }
.code > .tg-btn { background: var(--tg-bg); border-radius: 0; }
.kinds { border: 0; border-block: 1px solid var(--tg-sep); margin: 0; padding: 12px 16px; display: flex; flex-wrap: wrap; gap: 12px; }
.col { display: flex; flex-direction: column; gap: 1px; background: var(--tg-sep); }
.check { background: var(--tg-bg); padding: 12px 16px; }
</style>
