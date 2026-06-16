<script setup>
// Страница лабы №12 — авто-зачёт по файлу успеваемости.
// Админ загружает .xlsx → бэкенд возвращает отчёт по группам; здесь он
// визуализируется: сводка, таблицы студентов с процентами и бейджем
// «зачёт/незачёт», разворачиваемый список занятий, список зачётников.
import { ref, computed } from 'vue'
import AppHeader from '../components/AppHeader.vue'
import AppSidebar from '../components/AppSidebar.vue'
import { attendanceApi } from '../api/attendance'

const sidebarOpen = ref(false)

const fileInput = ref(null)
const fileName = ref('')
const loading = ref(false)
const error = ref('')
const report = ref(null) // { groups: [...], automatic_success_students: [...] }
const expanded = ref({}) // ключ "группа|ФИО" → развёрнут ли список занятий

// Человеческие тексты под коды ошибок бэкенда (handler/attendance.go).
const ERROR_TEXT = {
  invalid_file_type: 'Файл должен быть в формате .xlsx (книга Excel).',
  invalid_file: 'Файл не приложен или повреждён. Выберите .xlsx и попробуйте снова.',
  invalid_body: 'Не удалось обработать загрузку. Выберите файл и попробуйте снова.',
  sheet_not_found: 'В книге нет листа «Посещаемость» — проверьте имя листа.',
  missing_columns: 'В файле не хватает обязательных колонок.',
  invalid_row: 'В файле есть некорректная строка — проверьте данные.',
  too_many_rows: 'В файле слишком много строк — превышен лимит обработки.',
  file_too_large: 'Файл слишком большой.',
  forbidden: 'Недостаточно прав: нужно разрешение «calculate-attendance».',
}

const totalSuccess = computed(() =>
  (report.value?.groups ?? []).reduce((s, g) => s + g.result.success, 0),
)
const totalFail = computed(() =>
  (report.value?.groups ?? []).reduce((s, g) => s + g.result.unsuccessfully, 0),
)
const totalStudents = computed(() => totalSuccess.value + totalFail.value)

function pickFile() {
  fileInput.value?.click()
}

async function onFileChange(e) {
  const file = e.target.files?.[0]
  if (file) await submit(file)
  e.target.value = '' // позволяем выбрать тот же файл повторно
}

async function onDrop(e) {
  const file = e.dataTransfer?.files?.[0]
  if (file) await submit(file)
}

async function submit(file) {
  fileName.value = file.name
  error.value = ''
  report.value = null
  loading.value = true
  try {
    const { data } = await attendanceApi.calculate(file)
    report.value = data
  } catch (err) {
    const code = err.response?.data?.error
    // Детализированное серверное сообщение (invalid_row несёт номер строки) —
    // показываем его, иначе берём наш справочник, иначе общий текст.
    const serverMsg = err.response?.data?.message
    error.value =
      (code === 'invalid_row' && serverMsg) ||
      ERROR_TEXT[code] ||
      serverMsg ||
      'Не удалось обработать файл. Попробуйте ещё раз.'
  } finally {
    loading.value = false
  }
}

function toggle(group, name) {
  const key = `${group}|${name}`
  expanded.value[key] = !expanded.value[key]
}
function isExpanded(group, name) {
  return !!expanded.value[`${group}|${name}`]
}
function typeLabel(t) {
  return t === 'lect' ? 'Лекция' : t === 'lab' ? 'Лаб.' : t
}
</script>

<template>
  <div class="att-root">
    <AppSidebar :open="sidebarOpen" @close="sidebarOpen = false" />

    <div class="page-wrap">
      <AppHeader @open-sidebar="sidebarOpen = true" />

      <main class="main">
        <h1 class="title">Авто-зачёт по файлу успеваемости</h1>
        <p class="subtitle">
          Загрузите файл <b>.xlsx</b> с листом «Посещаемость» — система посчитает
          посещаемость и сданные лабораторные и определит, кто получает зачёт.
        </p>

        <!-- Зона загрузки -->
        <div
          class="dropzone"
          :class="{ loading }"
          @click="pickFile"
          @dragover.prevent
          @drop.prevent="onDrop"
        >
          <input
            ref="fileInput"
            type="file"
            accept=".xlsx"
            class="hidden-input"
            @change="onFileChange"
          />
          <svg class="dz-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/>
          </svg>
          <div class="dz-text">
            <template v-if="loading">Обработка файла…</template>
            <template v-else>
              Перетащите сюда <b>.xlsx</b> или <span class="dz-link">выберите файл</span>
            </template>
          </div>
          <div v-if="fileName && !loading" class="dz-file">{{ fileName }}</div>
        </div>

        <!-- Ошибка -->
        <div v-if="error" class="alert alert--error">{{ error }}</div>

        <!-- Результат -->
        <template v-if="report">
          <!-- Сводка -->
          <div class="cards">
            <div class="card">
              <div class="card-label">Всего студентов</div>
              <div class="card-value">{{ totalStudents }}</div>
            </div>
            <div class="card card--ok">
              <div class="card-label">Получили зачёт</div>
              <div class="card-value">{{ totalSuccess }}</div>
            </div>
            <div class="card card--no">
              <div class="card-label">Без зачёта</div>
              <div class="card-value">{{ totalFail }}</div>
            </div>
            <div class="card">
              <div class="card-label">Групп</div>
              <div class="card-value">{{ report.groups.length }}</div>
            </div>
          </div>

          <!-- Группы -->
          <section v-for="g in report.groups" :key="g.group_name" class="group">
            <div class="group-head">
              <h2 class="group-name">Группа {{ g.group_name }}</h2>
              <span class="group-stat">
                зачёт: <b class="ok">{{ g.result.success }}</b> ·
                без зачёта: <b class="no">{{ g.result.unsuccessfully }}</b>
              </span>
            </div>

            <table class="tbl">
              <thead>
                <tr>
                  <th>ФИО</th>
                  <th>Подгр.</th>
                  <th>Посещаемость</th>
                  <th>Сдано лаб</th>
                  <th>Результат</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                <template v-for="s in g.students" :key="s.name">
                  <tr :class="{ 'row-ok': s.result, 'row-no': !s.result }">
                    <td class="fio">{{ s.name }}</td>
                    <td class="center">{{ s.subgroup }}</td>
                    <td>
                      <div class="bar">
                        <div class="bar-fill" :style="{ width: Math.min(s.visit_percent, 100) + '%' }" />
                        <span class="bar-text">{{ s.visit_percent }}%</span>
                      </div>
                    </td>
                    <td class="center">{{ s.success_labs }} ({{ s.success_labs_percent }}%)</td>
                    <td class="center">
                      <span class="badge" :class="s.result ? 'badge--ok' : 'badge--no'">
                        {{ s.result ? 'Зачёт' : 'Нет' }}
                      </span>
                    </td>
                    <td class="center">
                      <button class="link-btn" @click="toggle(g.group_name, s.name)">
                        {{ isExpanded(g.group_name, s.name) ? 'Скрыть' : 'Занятия' }}
                      </button>
                    </td>
                  </tr>
                  <tr v-if="isExpanded(g.group_name, s.name)" class="lessons-row">
                    <td colspan="6">
                      <div class="lessons">
                        <div
                          v-for="(l, i) in s.leasons"
                          :key="i"
                          class="lesson"
                          :class="l.visit ? 'lesson--visit' : 'lesson--miss'"
                          :title="l.visit ? 'Посетил' : 'Пропустил'"
                        >
                          {{ l.date }} {{ l.time }} · {{ typeLabel(l.type) }} №{{ l.number }}
                        </div>
                      </div>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </section>

          <!-- Зачётники списком -->
          <section v-if="report.automatic_success_students?.length" class="success-list">
            <h2 class="group-name">Получили зачёт ({{ report.automatic_success_students.length }})</h2>
            <ul>
              <li v-for="(s, i) in report.automatic_success_students" :key="i">
                <b>{{ s.group }}</b> — {{ s.name }}
              </li>
            </ul>
          </section>
        </template>
      </main>
    </div>
  </div>
</template>

<style scoped>
.att-root { display: flex; min-height: 100vh; background: #f4f6fb; }
.page-wrap { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.main { padding: 24px 32px; max-width: 1100px; width: 100%; margin: 0 auto; }
.title { font-size: 24px; font-weight: 700; margin: 0 0 4px; }
.subtitle { color: #5b6478; margin: 0 0 20px; }

.dropzone {
  border: 2px dashed #b8c0d6; border-radius: 14px; background: #fff;
  padding: 32px; text-align: center; cursor: pointer; transition: .15s;
  display: flex; flex-direction: column; align-items: center; gap: 10px;
}
.dropzone:hover { border-color: #6c5ce7; background: #faf9ff; }
.dropzone.loading { opacity: .7; cursor: progress; }
.hidden-input { display: none; }
.dz-icon { width: 40px; height: 40px; color: #6c5ce7; }
.dz-text { color: #3a4153; }
.dz-link { color: #6c5ce7; font-weight: 600; }
.dz-file { font-size: 13px; color: #5b6478; }

.alert { margin-top: 16px; padding: 12px 16px; border-radius: 10px; font-size: 14px; }
.alert--error { background: #fdecec; color: #c0392b; border: 1px solid #f5b7b1; }

.cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin: 22px 0; }
.card { background: #fff; border-radius: 12px; padding: 16px; box-shadow: 0 1px 3px rgba(0,0,0,.06); }
.card-label { font-size: 12px; text-transform: uppercase; color: #8a93a6; letter-spacing: .04em; }
.card-value { font-size: 28px; font-weight: 700; margin-top: 4px; }
.card--ok .card-value { color: #1e8e4f; }
.card--no .card-value { color: #c0392b; }

.group { background: #fff; border-radius: 12px; padding: 18px; margin-bottom: 18px; box-shadow: 0 1px 3px rgba(0,0,0,.06); }
.group-head { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 10px; flex-wrap: wrap; gap: 6px; }
.group-name { font-size: 18px; font-weight: 700; margin: 0; }
.group-stat { font-size: 14px; color: #5b6478; }
.ok { color: #1e8e4f; } .no { color: #c0392b; }

.tbl { width: 100%; border-collapse: collapse; font-size: 14px; }
.tbl th { text-align: left; padding: 8px 10px; color: #8a93a6; font-weight: 600; border-bottom: 2px solid #eef0f6; font-size: 12px; text-transform: uppercase; }
.tbl td { padding: 9px 10px; border-bottom: 1px solid #f0f2f8; }
.center { text-align: center; }
.fio { font-weight: 500; }
.row-ok { background: #f3fbf6; } .row-no { background: #fef6f5; }

.bar { position: relative; height: 18px; background: #eef0f6; border-radius: 9px; overflow: hidden; min-width: 120px; }
.bar-fill { position: absolute; inset: 0 auto 0 0; background: linear-gradient(90deg,#6c5ce7,#8e7cf0); }
.bar-text { position: relative; font-size: 12px; line-height: 18px; padding-left: 8px; color: #2a2f3a; font-weight: 600; }

.badge { padding: 3px 10px; border-radius: 999px; font-size: 12px; font-weight: 700; }
.badge--ok { background: #d6f5e3; color: #1e8e4f; }
.badge--no { background: #fbe0dd; color: #c0392b; }

.link-btn { background: none; border: none; color: #6c5ce7; cursor: pointer; font-size: 13px; padding: 0; }
.lessons-row td { background: #fafbff; }
.lessons { display: flex; flex-wrap: wrap; gap: 6px; padding: 4px 0; }
.lesson { font-size: 12px; padding: 3px 8px; border-radius: 6px; }
.lesson--visit { background: #e3f6ec; color: #1e8e4f; }
.lesson--miss { background: #f3f4f8; color: #97a0b3; text-decoration: line-through; }

.success-list { background: #fff; border-radius: 12px; padding: 18px; box-shadow: 0 1px 3px rgba(0,0,0,.06); }
.success-list ul { margin: 10px 0 0; padding-left: 18px; columns: 2; }
.success-list li { margin-bottom: 4px; }

@media (max-width: 720px) {
  .cards { grid-template-columns: repeat(2, 1fr); }
  .success-list ul { columns: 1; }
}
</style>
