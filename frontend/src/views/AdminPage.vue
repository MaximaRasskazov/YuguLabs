<script setup>
import { ref, reactive, computed, watch, onMounted, onUnmounted } from 'vue'
import AppHeader from '../components/AppHeader.vue'
import AppSidebar from '../components/AppSidebar.vue'
import http from '../api/http'
import { saveBlob, filenameFromDisposition } from '../utils/download'

const sidebarOpen = ref(false)

// id строки с открытым дропдауном выбора роли (стилизованный, не нативный).
// Меню телепортируется в <body> с position:fixed — иначе его обрезает
// .table-wrap { overflow:auto } у нижних строк («уходит под блок»).
const openRoleId = ref(null)
const roleMenu   = ref({ top: 0, left: 0, minWidth: 160 }) // координаты fixed-меню
function toggleRoleDropdown(id, ev) {
  if (openRoleId.value === id) { openRoleId.value = null; return }
  const rect = ev.currentTarget.getBoundingClientRect()
  const estH = ASSIGNABLE_ROLES.length * 40 + 8       // примерная высота меню
  const openUp = rect.bottom + 4 + estH > window.innerHeight && rect.top - estH - 4 > 0
  roleMenu.value = {
    top: openUp ? Math.round(rect.top - estH - 4) : Math.round(rect.bottom + 4),
    left: Math.round(rect.left),
    minWidth: Math.round(rect.width),
  }
  openRoleId.value = id
}
function closeRoleDropdown() { openRoleId.value = null }
function onAdminOutside(e) {
  // Меню теперь в body — учитываем и его класс, иначе клик по опции «снаружи».
  if (!e.target.closest('.role-select-wrap') && !e.target.closest('.role-select-dropdown')) {
    openRoleId.value = null
  }
}
// Скролл/ресайз сдвигают триггер — fixed-меню «отрывается», поэтому закрываем.
function onAdminReposition() { if (openRoleId.value !== null) openRoleId.value = null }
onMounted(() => {
  document.addEventListener('mousedown', onAdminOutside)
  window.addEventListener('scroll', onAdminReposition, true)
  window.addEventListener('resize', onAdminReposition)
})
onUnmounted(() => {
  document.removeEventListener('mousedown', onAdminOutside)
  window.removeEventListener('scroll', onAdminReposition, true)
  window.removeEventListener('resize', onAdminReposition)
})

// Выбор роли из стилизованного дропдауна.
function pickRole(u, slug) {
  closeRoleDropdown()
  changeRole(u, slug)
}

/* ─── State ──────────────────────────────────────────────────── */
const users     = ref([])
const total     = ref(0)
const loading   = ref(false)
const forbidden = ref(false)

const PAGE_SIZE   = 20
const searchQuery = ref('')
const filterRole  = ref('')   // '' = все, 'student', 'teacher', 'dean'
const currentPage = ref(1)

// id пользователя, для которого сейчас идёт смена роли — блокирует селект.
const savingId = ref(null)
// Краткое уведомление об успехе/ошибке.
const toast = ref(null)
// id пользователей, чьё фото не загрузилось (нет аватара → 404) — для них
// показываем инициалы. Set реактивен: .add() в @error перерисует ячейку.
const avatarFailed = ref(new Set())

/* ─── Роли ───────────────────────────────────────────────────── */
// Слаги ролей совпадают с backend (roles.slug): student/teacher/dean.
// admin намеренно не выдаём через UI — это привилегия выше уровня действий.
const ASSIGNABLE_ROLES = [
  { slug: 'STUDENT', label: 'Студент' },
  { slug: 'TEACHER', label: 'Преподаватель' },
  { slug: 'DEAN',    label: 'Деканат' },
]
const ROLE_LABEL = {
  STUDENT: 'Студент', TEACHER: 'Преподаватель', DEAN: 'Деканат', ADMIN: 'Администратор',
}

const ROLE_STYLE = {
  DEAN:    { bg: 'rgba(217,119,6,.1)',  color: '#b45309' },
  TEACHER: { bg: 'rgba(139,61,240,.1)', color: '#7c22d6' },
  STUDENT: { bg: 'rgba(59,63,224,.1)',  color: '#3b3fe0' },
  ADMIN:   { bg: 'rgba(16,185,129,.1)', color: '#065f46' },
}
const DEFAULT_ROLE_STYLE = { bg: 'rgba(107,114,128,.1)', color: '#374151' }
function roleStyle(role) { return ROLE_STYLE[role] ?? DEFAULT_ROLE_STYLE }

/* ─── Fetch ──────────────────────────────────────────────────── */
async function fetchUsers() {
  loading.value = true
  forbidden.value = false
  try {
    const params = {
      limit:  PAGE_SIZE,
      offset: (currentPage.value - 1) * PAGE_SIZE,
    }
    if (filterRole.value)         params.role   = filterRole.value
    if (searchQuery.value.trim()) params.search = searchQuery.value.trim()

    const { data } = await http.get('/api/users', { params })
    users.value = (data.items ?? []).map(normalizeUser)
    total.value = data.total ?? 0
  } catch (e) {
    if (e.response?.status === 403) forbidden.value = true
    users.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function normalizeUser(u) {
  const role = (u.roles?.[0]?.slug ?? 'student').toUpperCase()
  return {
    id:         u.id,
    role,
    lastName:   u.last_name   ?? '',
    firstName:  u.first_name  ?? '',
    middleName: u.middle_name ?? '',
    group:      u.group_name  ?? null,
    email:      u.email,
  }
}

/* ─── Смена роли ─────────────────────────────────────────────── */
// Бэк хранит несколько ролей, но в этой системе у пользователя одна
// «основная». Меняем: сначала выдаём новую (POST), затем снимаем старую
// (DELETE), чтобы пользователь ни на миг не остался без роли.
async function changeRole(user, newRole) {
  if (newRole === user.role) return
  const prevRole = user.role
  savingId.value = user.id
  toast.value = null
  // Смена роли — ОДИН атомарный запрос: бэк в одной транзакции снимает старую
  // роль и выдаёт новую (POST /roles/change). Промежуточного состояния
  // «обе роли» нет; при ошибке на сервере не меняется ничего.
  try {
    await http.post(`/api/users/${user.id}/roles/change`, {
      from_slug: prevRole ? prevRole.toLowerCase() : '',
      to_slug: newRole.toLowerCase(),
    })
    user.role = newRole
    showToast('ok', `Роль изменена: ${fullName(user)} → ${ROLE_LABEL[newRole]}`)
  } catch (e) {
    const code = e.response?.data?.error
    const msg = code === 'privilege_escalation'
      ? 'Нельзя выдать роль выше своего уровня'
      : e.response?.data?.message || 'Не удалось изменить роль'
    showToast('err', msg)
    user.role = prevRole // ничего не применилось — возвращаем карточку
  } finally {
    savingId.value = null
  }
}

let toastTimer = null
function showToast(kind, text) {
  toast.value = { kind, text }
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.value = null }, 3500)
}

/* ─── Watchers ───────────────────────────────────────────────── */
watch(filterRole, () => { currentPage.value = 1; fetchUsers() })
watch(currentPage, fetchUsers)

let searchTimer = null
watch(searchQuery, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => { currentPage.value = 1; fetchUsers() }, 350)
})

onMounted(fetchUsers)

/* ─── Pagination ─────────────────────────────────────────────── */
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const rangeStart = computed(() => total.value === 0 ? 0 : (currentPage.value - 1) * PAGE_SIZE + 1)
const rangeEnd   = computed(() => Math.min(currentPage.value * PAGE_SIZE, total.value))

const visiblePages = computed(() => {
  const t = totalPages.value, c = currentPage.value
  if (t <= 7) return Array.from({ length: t }, (_, i) => i + 1)
  const pages = [1]
  if (c > 3) pages.push('…')
  for (let i = Math.max(2, c - 1); i <= Math.min(t - 1, c + 1); i++) pages.push(i)
  if (c < t - 2) pages.push('…')
  pages.push(t)
  return pages
})

function goPage(p) {
  if (typeof p !== 'number') return
  currentPage.value = Math.max(1, Math.min(totalPages.value, p))
}

/* ─── Helpers ────────────────────────────────────────────────── */
function fullName(u) {
  return [u.lastName, u.firstName, u.middleName].filter(Boolean).join(' ')
}

/* ─── Правка данных пользователя (админ) ─────────────────────── */
// PATCH /api/users/{id}: правка ФИО/группы. Логируется на бэке в change_logs
// с автором-администратором — изменение видно в «Истории» и откатываемо.
const editOpen   = ref(false)
const editUser   = ref(null)
const editSaving = ref(false)
const editForm   = reactive({ lastName: '', firstName: '', middleName: '', group: '' })

function openEdit(u) {
  editUser.value   = u
  editForm.lastName   = u.lastName   ?? ''
  editForm.firstName  = u.firstName  ?? ''
  editForm.middleName = u.middleName ?? ''
  editForm.group      = u.group      ?? ''
  editOpen.value = true
}
function closeEdit() { editOpen.value = false; editUser.value = null }

async function saveEdit() {
  if (!editForm.firstName.trim() || !editForm.lastName.trim()) {
    showToast('err', 'Имя и фамилия обязательны')
    return
  }
  editSaving.value = true
  try {
    const { data } = await http.patch(`/api/users/${editUser.value.id}`, {
      first_name:  editForm.firstName.trim(),
      last_name:   editForm.lastName.trim(),
      middle_name: editForm.middleName.trim(),
      group_name:  editForm.group.trim(),
    })
    const row = users.value.find((x) => x.id === editUser.value.id)
    if (row) {
      row.lastName   = data.last_name   ?? ''
      row.firstName  = data.first_name  ?? ''
      row.middleName = data.middle_name ?? ''
      row.group      = data.group_name  ?? null
    }
    showToast('ok', `Данные обновлены: ${fullName(editForm)}`)
    closeEdit()
  } catch (e) {
    showToast('err', e.response?.data?.message || 'Не удалось обновить данные')
  } finally {
    editSaving.value = false
  }
}

const AVATAR_PALETTE = [
  '#3b3fe0','#e63c5a','#f59e0b','#10b981',
  '#8b3df0','#0ea5e9','#ec4899','#14b8a6',
  '#f97316','#6366f1','#84cc16','#06b6d4',
]
function avatarBg(u) {
  const key = u.lastName + u.firstName
  let h = 0
  for (const c of key) h = ((h << 5) - h + c.charCodeAt(0)) >>> 0
  return AVATAR_PALETTE[h % AVATAR_PALETTE.length]
}
function initials(u) {
  return ((u.lastName?.[0] ?? '') + (u.firstName?.[0] ?? '')).toUpperCase()
}

/* ─── История изменений (audit) ──────────────────────────────── */
const ACTION_LABEL = {
  created:           'Создан',
  updated:           'Профиль изменён',
  role_assigned:     'Выдана роль',
  role_revoked:      'Снята роль',
  role_changed:      'Роль изменена',
  password_changed:  'Сменён пароль',
  restored_from_log: 'Откат из истории',
  soft_deleted:      'Удалён',
  restored:          'Восстановлен',
}
const FIELD_LABEL = {
  first_name: 'Имя', last_name: 'Фамилия', middle_name: 'Отчество',
  group_name: 'Группа', birthday: 'Дата рождения', email: 'Почта',
}
// Действия, для которых бэкенд умеет откат (см. changelog.RestoreFromLog).
const RESTORABLE = new Set(['updated', 'role_assigned', 'role_revoked', 'role_changed'])

const historyUser    = ref(null) // пользователь, чья история открыта (null = модалка закрыта)
const historyEntries = ref([])
const historyLoading = ref(false)
const restoringId    = ref(null)
// id записей, чей откат уже выполнен в этой сессии — их кнопка становится
// серым бейджем «Откат выполнен» (повторный откат той же записи — no-op).
const restoredIds    = ref(new Set())

async function openHistory(u) {
  historyUser.value = u
  historyEntries.value = []
  historyLoading.value = true
  try {
    const { data } = await http.get(`/api/users/${u.id}/story`, { params: { limit: 100 } })
    historyEntries.value = (Array.isArray(data) ? data : []).map(describeEntry)
  } catch (e) {
    showToast('err', e.response?.status === 403 ? 'Нет права на просмотр истории' : 'Не удалось загрузить историю')
    historyUser.value = null
  } finally {
    historyLoading.value = false
  }
}
function closeHistory() { historyUser.value = null; historyEntries.value = [] }

// describeEntry превращает запись лога в человекочитаемый вид:
//   title   — действие, для ролей сразу с названием («Выдана роль «Деканат»»);
//   details — только понятные поля профиля (old→new), без технических ключей;
//   subject — ФИО затронутого пользователя (в журнале).
// currentFieldValue возвращает текущее значение поля у пользователя, чью
// историю мы смотрим (historyUser). Нужно, чтобы понять, в силе ли ещё
// изменение. Несопоставимые поля (role_name, технические) → undefined.
function currentFieldValue(field) {
  const u = historyUser.value
  if (!u) return undefined
  switch (field) {
    case 'role_slug':   return (u.role ?? '').toLowerCase()
    case 'first_name':  return u.firstName ?? ''
    case 'last_name':   return u.lastName ?? ''
    case 'middle_name': return u.middleName ?? ''
    case 'group_name':  return u.group ?? ''
    default:            return undefined
  }
}

// entryStillInEffect сообщает, имеет ли смысл откат записи: true, если текущее
// состояние всё ещё совпадает с «new» этой записи (изменение в силе). Это
// вычисляется из АКТУАЛЬНЫХ данных пользователя, поэтому корректно и после
// перезагрузки страницы (когда сессионный restoredIds потерян): уже
// откаченная запись не предложит кнопку повторно. Если сопоставимых полей нет
// — оставляем кнопку (бэкенд всё равно безопасно вернёт restore_noop).
function entryStillInEffect(cf) {
  let sawComparable = false
  for (const [field, ch] of Object.entries(cf)) {
    const cur = currentFieldValue(field)
    if (cur === undefined) continue
    const newVal = ch?.new
    if (newVal === null || newVal === undefined || newVal === '') continue
    sawComparable = true
    if (String(cur) === String(newVal)) return true
  }
  return !sawComparable
}

function describeEntry(e) {
  const cf = e.changed_fields || {}
  let title = ACTION_LABEL[e.action] ?? e.action
  // Название роли, если запись про роль (в т.ч. в записи отката).
  const roleRef = cf.role_name ?? cf.role_slug ?? {}
  const roleName = roleRef.new ?? roleRef.old
  if (e.action === 'role_assigned' || e.action === 'role_revoked') {
    if (roleName) title += ` «${roleName}»`
  } else if (e.action === 'role_changed') {
    // Смена роли: before→after. Если прежней роли не было — просто «выдана».
    title = roleRef.old
      ? `Роль изменена: «${roleRef.old}» → «${roleRef.new}»`
      : `Выдана роль «${roleRef.new}»`
  } else if (e.action === 'restored_from_log') {
    // Поясняем, ЧТО именно откатили: роль или профиль.
    title = roleName ? `Откат: роль «${roleName}»` : 'Откат изменения профиля'
  }
  // Подробности — для правки профиля и отката профиля: только понятные поля
  // (технические ключи и UUID просто отсутствуют в FIELD_LABEL).
  const details = (e.action === 'updated' || e.action === 'restored_from_log')
    ? Object.entries(cf)
        .filter(([k]) => FIELD_LABEL[k])
        .map(([k, v]) => ({ label: FIELD_LABEL[k], old: v.old, new: v.new }))
    : []
  return {
    id: e.id,
    title,
    at: e.created_at,
    details,
    // Откатываемо, только если действие в принципе поддерживает откат И
    // изменение всё ещё в силе (иначе кнопка повторного «пустого» отката не
    // показывается — устранён баг с её появлением после перезагрузки).
    restorable: RESTORABLE.has(e.action) && entryStillInEffect(cf),
    subject: e.subject || '',
  }
}

async function restoreEntry(entry) {
  restoringId.value = entry.id
  try {
    await http.post(`/api/changelog/${entry.id}/restore`)
    restoredIds.value.add(entry.id) // кнопка станет бейджем «Откат выполнен»
    showToast('ok', 'Откат выполнен')
    // Сначала обновляем список, затем берём СВЕЖИЙ объект пользователя и
    // перезагружаем историю — чтобы restorable пересчитался по актуальному
    // состоянию (роль/профиль уже изменились).
    await fetchUsers()
    const fresh = users.value.find((u) => u.id === historyUser.value?.id)
    if (fresh) historyUser.value = fresh
    await openHistory(historyUser.value)
  } catch (e) {
    const code = e.response?.data?.error
    // Бэкенд присылает информативное сообщение — показываем его как есть.
    const serverMsg = e.response?.data?.message
    if (code === 'restore_noop') {
      // Состояние уже соответствует записи — это не ошибка, просто гасим кнопку.
      restoredIds.value.add(entry.id)
      showToast('ok', serverMsg || 'Откат уже выполнен')
      return
    }
    showToast('err', serverMsg || 'Не удалось выполнить откат')
  } finally {
    restoringId.value = null
  }
}

/* ─── Общий журнал изменений ─────────────────────────────────── */
const journalOpen    = ref(false)
const journalEntries = ref([])
const journalLoading = ref(false)

async function openJournal() {
  journalOpen.value = true
  journalEntries.value = []
  journalLoading.value = true
  try {
    const { data } = await http.get('/api/changelog', { params: { limit: 100 } })
    journalEntries.value = (Array.isArray(data) ? data : []).map(describeEntry)
  } catch (e) {
    showToast('err', e.response?.status === 403 ? 'Нет права на просмотр журнала' : 'Не удалось загрузить журнал')
    journalOpen.value = false
  } finally {
    journalLoading.value = false
  }
}
function closeJournal() { journalOpen.value = false; journalEntries.value = [] }

/* ─── Архив фотографий (ZIP + Excel-реестр) ──────────────────── */
const archiving = ref(false)
async function downloadArchive() {
  if (archiving.value) return
  archiving.value = true
  try {
    const resp = await http.post('/api/photo/archive', null, { responseType: 'blob' })
    const name = filenameFromDisposition(resp.headers['content-disposition'], 'user-photos.zip')
    saveBlob(resp.data, name)
    showToast('ok', 'Архив фотографий скачан')
  } catch (e) {
    showToast('err', e.response?.status === 403 ? 'Нет права на выгрузку архива' : 'Не удалось собрать архив')
  } finally {
    archiving.value = false
  }
}

function formatVal(v) {
  return (v === null || v === undefined || v === '') ? '—' : String(v)
}
function fmtDateTime(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleString('ru-RU', {
    day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}
</script>

<template>
  <div class="admin-root">
    <AppSidebar :open="sidebarOpen" @close="sidebarOpen = false" />

    <div class="page-admin">
      <AppHeader @open-sidebar="sidebarOpen = true" />

      <!-- ── Заголовок ── -->
      <div class="page-bar">
        <div class="page-bar-left">
          <h1 class="page-title">Управление ролями</h1>
          <span v-if="total > 0" class="total-chip">{{ total }} в системе</span>
        </div>
        <div class="page-bar-actions">
          <button class="journal-btn" @click="downloadArchive" :disabled="archiving" title="ZIP всех фотографий + Excel-реестр">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 10l5 5 5-5"/><path d="M12 15V3"/>
            </svg>
            {{ archiving ? 'Готовим…' : 'Архив фото' }}
          </button>
          <button class="journal-btn" @click="openJournal" title="Последние изменения по всем">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M3 3v5h5"/><path d="M3.05 13A9 9 0 1 0 6 5.3L3 8"/><path d="M12 7v5l3 2"/>
            </svg>
            Журнал изменений
          </button>
        </div>
      </div>

      <!-- ── Фильтры ── -->
      <div class="filters-bar">
        <div class="search-wrap">
          <svg class="search-icon" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/>
          </svg>
          <input
            class="search-input"
            type="text"
            v-model="searchQuery"
            placeholder="Поиск по ФИО, почте или группе..."
          />
          <button v-if="searchQuery" class="search-clear" @click="searchQuery = ''">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
          </button>
        </div>

        <div class="chips">
          <button class="chip" :class="{ active: filterRole === '' }" @click="filterRole = ''">Все</button>
          <button class="chip" :class="{ active: filterRole === 'student' }" @click="filterRole = 'student'">Студенты</button>
          <button class="chip" :class="{ active: filterRole === 'teacher' }" @click="filterRole = 'teacher'">Преподаватели</button>
          <button class="chip" :class="{ active: filterRole === 'dean' }" @click="filterRole = 'dean'">Деканат</button>
        </div>
      </div>

      <!-- ── Счётчик ── -->
      <div class="results-bar">
        <span v-if="loading" class="results-text results-empty">Загрузка…</span>
        <span v-else-if="total > 0" class="results-text">
          Показано&nbsp;<strong>{{ rangeStart }}–{{ rangeEnd }}</strong>&nbsp;из&nbsp;<strong>{{ total }}</strong>
        </span>
        <span v-else-if="!forbidden" class="results-text results-empty">Никого не найдено</span>
      </div>

      <!-- ── 403 ── -->
      <div v-if="forbidden" class="empty-state">
        <svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="#d7d9e0" stroke-width="1.2" stroke-linecap="round">
          <circle cx="12" cy="12" r="10"/><line x1="4.93" y1="4.93" x2="19.07" y2="19.07"/>
        </svg>
        <p class="empty-title">Нет доступа</p>
        <p class="empty-sub">Управление ролями доступно только администратору</p>
      </div>

      <!-- ── Таблица пользователей ── -->
      <div class="table-card" v-else-if="!loading && users.length">
        <div class="table-wrap">
          <table class="data-table">
            <thead>
              <tr class="head-row">
                <th>Пользователь</th>
                <th>Почта</th>
                <th>Группа</th>
                <th>Текущая роль</th>
                <th>Изменить роль</th>
                <th>Действия</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="u in users" :key="u.id">
                <td>
                  <div class="user-cell">
                    <div class="u-avatar" :style="{ background: avatarBg(u) }">
                      <span class="u-initials">{{ initials(u) }}</span>
                      <img
                        v-if="!avatarFailed.has(u.id)"
                        class="u-avatar-img"
                        :src="`/api/users/${u.id}/avatar`"
                        alt="" loading="lazy"
                        @error="avatarFailed.add(u.id)"
                      />
                    </div>
                    <span class="u-name">{{ fullName(u) }}</span>
                  </div>
                </td>
                <td class="td-soft">{{ u.email }}</td>
                <td class="td-soft">{{ u.group || '—' }}</td>
                <td>
                  <span class="u-badge" :style="{ background: roleStyle(u.role).bg, color: roleStyle(u.role).color }">
                    {{ ROLE_LABEL[u.role] ?? u.role }}
                  </span>
                </td>
                <td>
                  <div class="role-select-wrap" :class="{ open: openRoleId === u.id }">
                    <button
                      type="button"
                      class="role-select-trigger"
                      :disabled="savingId === u.id || u.role === 'ADMIN'"
                      @click="toggleRoleDropdown(u.id, $event)"
                    >
                      <span>{{ ROLE_LABEL[u.role] ?? u.role }}</span>
                      <svg class="role-select-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="6 9 12 15 18 9"/></svg>
                    </button>
                    <Teleport to="body">
                      <div
                        v-if="openRoleId === u.id"
                        class="role-select-dropdown role-select-dropdown--fixed"
                        :style="{ top: roleMenu.top + 'px', left: roleMenu.left + 'px', minWidth: roleMenu.minWidth + 'px' }"
                      >
                        <button
                          v-for="r in ASSIGNABLE_ROLES" :key="r.slug"
                          type="button" class="role-select-option"
                          :class="{ selected: u.role === r.slug }"
                          @click="pickRole(u, r.slug)"
                        >{{ r.label }}</button>
                      </div>
                    </Teleport>
                    <span v-if="savingId === u.id" class="select-spinner" />
                  </div>
                </td>
                <td>
                  <div class="row-actions">
                    <button type="button" class="hist-btn" title="Изменить ФИО / группу" @click="openEdit(u)">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>
                      </svg>
                      Изменить
                    </button>
                    <button type="button" class="hist-btn" title="История изменений" @click="openHistory(u)">
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M3 3v5h5"/><path d="M3.05 13A9 9 0 1 0 6 5.3L3 8"/><path d="M12 7v5l3 2"/>
                      </svg>
                      История
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Пустое состояние -->
      <div v-else-if="!loading && !forbidden" class="empty-state">
        <svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="#d7d9e0" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/>
        </svg>
        <p class="empty-title">Никого не найдено</p>
        <p class="empty-sub">Попробуйте изменить запрос или сбросить фильтры</p>
        <button class="btn-reset" @click="searchQuery = ''; filterRole = ''">Сбросить фильтры</button>
      </div>

      <!-- ── Пагинация ── -->
      <div v-if="totalPages > 1" class="pagination">
        <button class="pg-btn" :disabled="currentPage === 1" @click="goPage(currentPage - 1)" title="Предыдущая">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M15 18l-6-6 6-6"/></svg>
        </button>
        <template v-for="p in visiblePages" :key="p + '-' + currentPage">
          <span v-if="p === '…'" class="pg-dots">…</span>
          <button v-else class="pg-btn" :class="{ active: p === currentPage }" @click="goPage(p)">{{ p }}</button>
        </template>
        <button class="pg-btn" :disabled="currentPage === totalPages" @click="goPage(currentPage + 1)" title="Следующая">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M9 18l6-6-6-6"/></svg>
        </button>
      </div>

      <div class="page-bottom-space" />
    </div>

    <!-- ── История изменений ── -->
    <Transition name="modal">
      <div v-if="historyUser" class="modal-overlay" @click.self="closeHistory">
        <div class="modal-card">
          <div class="modal-head">
            <div class="modal-head-text">
              <h3 class="modal-title">История изменений</h3>
              <span class="modal-sub">{{ fullName(historyUser) }}</span>
            </div>
            <button class="modal-close" @click="closeHistory" aria-label="Закрыть">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
            </button>
          </div>
          <div class="modal-body">
            <p v-if="!historyLoading && historyEntries.length" class="modal-hint">
              «Откатить» вернёт состояние на момент <b>до</b> выбранного изменения.
            </p>
            <div v-if="historyLoading" class="modal-state">Загрузка…</div>
            <div v-else-if="!historyEntries.length" class="modal-state">Изменений пока нет</div>
            <ul v-else class="hist-list">
              <li v-for="e in historyEntries" :key="e.id" class="hist-item">
                <div class="hist-item-head">
                  <span class="hist-action">{{ e.title }}</span>
                  <span class="hist-time">{{ fmtDateTime(e.at) }}</span>
                </div>
                <div v-if="e.details.length" class="hist-fields">
                  <div v-for="(ln, i) in e.details" :key="i" class="hist-field">
                    <span class="hist-field-label">{{ ln.label }}:</span>
                    <span class="hist-val old">{{ formatVal(ln.old) }}</span>
                    <svg class="hist-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
                    <span class="hist-val new">{{ formatVal(ln.new) }}</span>
                  </div>
                </div>
                <!-- По ТЗ: откат к состоянию из КОНКРЕТНОЙ записи (её "before").
                     Доступен на каждой откатываемой записи. После отката кнопка
                     превращается в серый бейдж — повторный откат той же записи
                     ничего не меняет. -->
                <div v-if="e.restorable" class="hist-item-foot">
                  <button v-if="!restoredIds.has(e.id)" class="hist-restore" :disabled="restoringId === e.id" @click="restoreEntry(e)">
                    {{ restoringId === e.id ? 'Откат…' : 'Откатить' }}
                  </button>
                  <span v-else class="hist-restored-badge" title="Откат этой записи уже выполнен">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6L9 17l-5-5"/></svg>
                    Откат выполнен
                  </span>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Transition>

    <!-- ── Журнал изменений (все сущности) ── -->
    <Transition name="modal">
      <div v-if="journalOpen" class="modal-overlay" @click.self="closeJournal">
        <div class="modal-card">
          <div class="modal-head">
            <div class="modal-head-text">
              <h3 class="modal-title">Журнал изменений</h3>
              <span class="modal-sub">последние действия по всем пользователям и ролям</span>
            </div>
            <button class="modal-close" @click="closeJournal" aria-label="Закрыть">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
            </button>
          </div>
          <div class="modal-body">
            <div v-if="journalLoading" class="modal-state">Загрузка…</div>
            <div v-else-if="!journalEntries.length" class="modal-state">Журнал пуст</div>
            <ul v-else class="hist-list">
              <li v-for="e in journalEntries" :key="e.id" class="hist-item">
                <div class="hist-item-head">
                  <span class="hist-action">{{ e.title }}</span>
                  <span class="hist-time">{{ fmtDateTime(e.at) }}</span>
                </div>
                <div v-if="e.subject" class="hist-subject">{{ e.subject }}</div>
                <div v-if="e.details.length" class="hist-fields">
                  <div v-for="(ln, i) in e.details" :key="i" class="hist-field">
                    <span class="hist-field-label">{{ ln.label }}:</span>
                    <span class="hist-val old">{{ formatVal(ln.old) }}</span>
                    <svg class="hist-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg>
                    <span class="hist-val new">{{ formatVal(ln.new) }}</span>
                  </div>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Transition>

    <!-- ── Правка данных пользователя (админ) ── -->
    <Transition name="modal">
      <div v-if="editOpen" class="modal-overlay" @click.self="closeEdit">
        <div class="modal-card modal-card--narrow">
          <div class="modal-head">
            <div class="modal-head-text">
              <h3 class="modal-title">Изменить данные</h3>
              <span class="modal-sub">{{ editUser?.email }}</span>
            </div>
            <button class="modal-close" @click="closeEdit" aria-label="Закрыть">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6L6 18M6 6l12 12"/></svg>
            </button>
          </div>
          <div class="modal-body">
            <div class="edit-grid">
              <label class="edit-field"><span>Фамилия</span><input class="edit-input" v-model="editForm.lastName" placeholder="Фамилия" /></label>
              <label class="edit-field"><span>Имя</span><input class="edit-input" v-model="editForm.firstName" placeholder="Имя" /></label>
              <label class="edit-field"><span>Отчество</span><input class="edit-input" v-model="editForm.middleName" placeholder="—" /></label>
              <label class="edit-field"><span>Группа</span><input class="edit-input" v-model="editForm.group" placeholder="Напр. ИВТ-21" /></label>
            </div>
          </div>
          <div class="edit-foot">
            <button class="edit-btn-cancel" @click="closeEdit">Отмена</button>
            <button class="edit-btn-save" :disabled="editSaving" @click="saveEdit">
              {{ editSaving ? 'Сохранение…' : 'Сохранить' }}
            </button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- ── Toast ── -->
    <Transition name="toast">
      <div v-if="toast" class="toast" :class="`toast--${toast.kind}`">
        <svg v-if="toast.kind === 'ok'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
        <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
        <span>{{ toast.text }}</span>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');

*, *::before, *::after { box-sizing: border-box; }

.admin-root {
  --bg:        #f3f4f7;
  --card:      #ffffff;
  --ink:       #1a1d24;
  --ink-soft:  #6b7280;
  --line:      #d7d9e0;
  --brand:     #3b3fe0;
  --brand-ink: #2a2e9e;
  --radius:    10px;
  --shadow:    0 2px 8px rgba(20,22,60,.07);
  --ease:      cubic-bezier(.2,.7,.2,1);

  min-height: 100dvh;
  font-family: 'Inter', system-ui, sans-serif;
  color: var(--ink);
  -webkit-font-smoothing: antialiased;
}

.page-admin {
  min-height: 100dvh; background: var(--bg);
  display: flex; flex-direction: column;
}

/* ── Page bar ── */
.page-bar { display: flex; align-items: center; gap: 12px; padding: 20px 24px 0; }
.page-bar-left { display: flex; align-items: center; gap: 12px; }
.page-title {
  font-family: 'Gerhaus', 'Inter', sans-serif;
  font-size: 20px; font-weight: 700; color: #3C38B6; margin: 0;
}
.total-chip {
  padding: 4px 12px; border-radius: 20px;
  background: rgba(59,63,224,.1); color: var(--brand);
  font: 600 12px/1 'Inter', sans-serif;
}

/* ── Filters ── */
.filters-bar { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; padding: 16px 24px 0; }
.search-wrap { position: relative; flex: 1; min-width: 220px; max-width: 380px; }
.search-icon { position: absolute; left: 11px; top: 50%; transform: translateY(-50%); color: var(--ink-soft); pointer-events: none; }
.search-input {
  width: 100%; height: 38px;
  border: 1.5px solid var(--line); border-radius: var(--radius);
  background: var(--card); padding: 0 34px;
  font: 13px/1 'Inter', sans-serif; color: var(--ink); outline: none;
  transition: border-color .2s var(--ease), box-shadow .2s var(--ease);
}
.search-input:focus { border-color: var(--brand); box-shadow: 0 0 0 3px rgba(59,63,224,.1); }
.search-input::placeholder { color: #b0b3be; }
.search-clear {
  position: absolute; right: 10px; top: 50%; transform: translateY(-50%);
  border: none; background: none; color: var(--ink-soft); cursor: pointer;
  display: grid; place-items: center; width: 20px; height: 20px; border-radius: 4px; padding: 0;
  transition: background .15s, color .15s;
}
.search-clear:hover { background: rgba(220,38,38,.08); color: #dc2626; }

.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.chip {
  display: flex; align-items: center; gap: 5px;
  padding: 0 12px; height: 34px; border: 1.5px solid var(--line); border-radius: 20px;
  background: var(--card); color: var(--ink-soft);
  font: 500 13px/1 'Inter', sans-serif; cursor: pointer; white-space: nowrap;
  transition: border-color .15s, background .15s, color .15s;
}
.chip:hover { border-color: var(--brand); color: var(--brand); }
.chip.active { background: rgba(59,63,224,.1); border-color: var(--brand); color: var(--brand); font-weight: 600; }

/* ── Results ── */
.results-bar { padding: 10px 24px 0; }
.results-text { font-size: 13px; color: var(--ink-soft); }
.results-text strong { color: var(--ink); font-weight: 600; }
.results-empty { color: #b0b3be; }

/* ── Table ── */
.table-card {
  margin: 16px 24px 0; background: var(--card);
  border-radius: var(--radius); box-shadow: var(--shadow); padding: 8px;
}
.table-wrap { overflow: auto; border-radius: 8px; }
.data-table { width: 100%; border-collapse: collapse; font: 13px/1.4 'Inter', sans-serif; }
.head-row th {
  position: sticky; top: 0; z-index: 2;
  background: #f8f9fb; padding: 12px 16px;
  font: 600 12px/1 'Inter', sans-serif; color: var(--ink-soft);
  text-align: left; white-space: nowrap; border-bottom: 1px solid var(--line);
}
.data-table tbody tr { transition: background .12s; }
.data-table tbody tr:hover { background: rgba(59,63,224,.03); }
.data-table td { padding: 12px 16px; border-bottom: 1px solid var(--line); color: var(--ink); vertical-align: middle; text-align: left; }
.data-table tbody tr:last-child td { border-bottom: none; }
.td-soft { color: var(--ink-soft); }

.user-cell { display: flex; align-items: center; gap: 12px; }
.u-avatar {
  width: 38px; height: 38px; border-radius: 50%; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  position: relative; overflow: hidden;
}
.u-initials { font: 700 13px/1 'Inter', sans-serif; color: #fff; text-shadow: 0 1px 2px rgba(0,0,0,.2); }
/* Фото поверх кружка с инициалами; при 404 (@error) <img> убирается — видны инициалы. */
.u-avatar-img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
.u-name { font-weight: 600; color: var(--ink); white-space: nowrap; }

.u-badge { padding: 3px 10px; border-radius: 20px; font: 600 11px/1 'Inter', sans-serif; white-space: nowrap; }

/* ── Role select (стилизованный дропдаун вместо нативного select) ── */
.role-select-wrap { position: relative; display: flex; align-items: center; gap: 8px; }
.role-select-trigger {
  display: flex; align-items: center; justify-content: space-between; gap: 8px;
  height: 36px; padding: 0 12px; min-width: 160px;
  border: 1.5px solid var(--line); border-radius: var(--radius);
  background: var(--card); color: var(--ink); text-align: left;
  font: 500 13px/1 'Inter', sans-serif; cursor: pointer; outline: none;
  transition: border-color .2s var(--ease), box-shadow .2s var(--ease);
}
.role-select-trigger:hover:not(:disabled) { border-color: #a0a3b1; }
.role-select-wrap.open .role-select-trigger { border-color: var(--brand); box-shadow: 0 0 0 3px rgba(59,63,224,.1); }
.role-select-trigger:disabled { opacity: .6; cursor: not-allowed; background-color: #f8f9fb; }
.role-select-arrow { width: 14px; height: 14px; color: var(--ink-soft); flex-shrink: 0; transition: transform .2s var(--ease); }
.role-select-wrap.open .role-select-arrow { transform: rotate(180deg); }
.role-select-dropdown {
  position: absolute; top: calc(100% + 4px); left: 0; min-width: 160px; z-index: 100;
  background: #fff; border: 1.5px solid var(--line); border-radius: 8px;
  box-shadow: 0 8px 24px -4px rgba(20,22,60,.14); padding: 4px;
}
/* Меню телепортировано в body — позиционируется fixed по координатам триггера,
   чтобы его не обрезал overflow таблицы. top/left/min-width задаются inline. */
.role-select-dropdown--fixed { position: fixed; z-index: 1000; }
.role-select-option {
  display: block; width: 100%; text-align: left;
  padding: 9px 12px; border: none; border-radius: 6px;
  background: none; font: 13px/1.3 'Inter', sans-serif; color: var(--ink);
  cursor: pointer; transition: background .12s; white-space: nowrap;
}
.role-select-option:hover { background: rgba(59,63,224,.07); }
.role-select-option.selected { color: var(--brand); font-weight: 600; background: rgba(59,63,224,.06); }
.select-spinner {
  width: 16px; height: 16px; border-radius: 50%; flex-shrink: 0;
  border: 2px solid var(--line); border-top-color: var(--brand);
  animation: spin .8s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }

/* ── Empty ── */
.empty-state { display: flex; flex-direction: column; align-items: center; padding: 60px 24px; gap: 10px; }
.empty-title { font-size: 16px; font-weight: 600; color: var(--ink); margin: 0; }
.empty-sub { font-size: 13px; color: var(--ink-soft); margin: 0; text-align: center; line-height: 1.6; }
.btn-reset {
  margin-top: 8px; padding: 0 20px; height: 36px; border-radius: var(--radius);
  border: 1.5px solid var(--line); background: var(--card); color: var(--ink);
  font: 500 13px/1 'Inter', sans-serif; cursor: pointer;
  transition: border-color .15s, color .15s;
}
.btn-reset:hover { border-color: var(--brand); color: var(--brand); }

/* ── Pagination ── */
.pagination { display: flex; align-items: center; justify-content: center; gap: 4px; padding: 24px 24px 0; }
.pg-btn {
  min-width: 36px; height: 36px; padding: 0 8px;
  border: 1.5px solid var(--line); border-radius: 8px;
  background: var(--card); color: var(--ink-soft);
  font: 500 13px/1 'Inter', sans-serif; cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  transition: border-color .15s, background .15s, color .15s;
}
.pg-btn:hover:not(:disabled) { border-color: var(--brand); color: var(--brand); background: rgba(59,63,224,.05); }
.pg-btn.active { background: var(--brand); border-color: var(--brand); color: #fff; font-weight: 700; }
.pg-btn:disabled { opacity: .35; cursor: not-allowed; }
.pg-dots { min-width: 36px; text-align: center; color: var(--ink-soft); font-size: 14px; }

.page-bottom-space { height: 32px; }

/* ── Toast ── */
.toast {
  position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%);
  z-index: 500; display: flex; align-items: center; gap: 10px;
  padding: 12px 18px; border-radius: 12px;
  font: 600 13px/1.4 'Inter', sans-serif;
  box-shadow: 0 12px 32px -8px rgba(10,12,30,.3);
  max-width: 90vw;
}
.toast svg { width: 18px; height: 18px; flex-shrink: 0; }
.toast--ok  { background: #065f46; color: #fff; }
.toast--err { background: #dc2626; color: #fff; }
.toast-enter-active, .toast-leave-active { transition: opacity .25s var(--ease), transform .25s var(--ease); }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translate(-50%, 12px); }

/* ── История: кнопка в строке ── */
.hist-btn {
  display: inline-flex; align-items: center; gap: 6px;
  height: 32px; padding: 0 12px; border-radius: 8px;
  border: 1.5px solid var(--line); background: var(--card);
  color: var(--ink-soft); font: 500 12px/1 'Inter', sans-serif; cursor: pointer; white-space: nowrap;
  transition: border-color .15s, color .15s, background .15s;
}
.hist-btn:hover { border-color: var(--brand); color: var(--brand); background: rgba(59,63,224,.05); }
.hist-btn svg { width: 14px; height: 14px; }

/* ── Кнопка «Журнал изменений» в шапке ── */
.page-bar-actions { margin-left: auto; display: flex; align-items: center; gap: 10px; }
.journal-btn {
  display: inline-flex; align-items: center; gap: 7px;
  height: 36px; padding: 0 14px; border-radius: 9px;
  border: 1.5px solid var(--brand); background: rgba(59,63,224,.06); color: var(--brand);
  font: 600 13px/1 'Inter', sans-serif; cursor: pointer; white-space: nowrap;
  transition: background .15s;
}
.journal-btn:hover:not(:disabled) { background: rgba(59,63,224,.12); }
.journal-btn:disabled { opacity: .6; cursor: default; }
.journal-btn svg { width: 15px; height: 15px; }
.hist-subject { margin-top: 5px; font: 600 13px/1.3 'Inter', sans-serif; color: var(--ink); }

/* ── История: модалка ── */
.modal-overlay {
  position: fixed; inset: 0; z-index: 400;
  background: rgba(10,12,30,.45);
  display: flex; align-items: center; justify-content: center; padding: 24px;
}
.modal-card {
  width: 100%; max-width: 560px; max-height: 82vh;
  background: var(--card); border-radius: 14px; overflow: hidden;
  display: flex; flex-direction: column;
  box-shadow: 0 24px 60px -16px rgba(10,12,30,.4);
}
.modal-head {
  position: relative;
  padding: 18px 48px; border-bottom: 1px solid var(--line);
  text-align: center;
}
.modal-head-text { display: flex; flex-direction: column; gap: 4px; align-items: center; }
.modal-title { margin: 0; font: 700 16px/1.2 'Inter', sans-serif; color: var(--ink); }
.modal-sub { font: 500 12px/1.4 'Inter', sans-serif; color: var(--ink-soft); }
.modal-close {
  position: absolute; top: 14px; right: 14px;
  width: 30px; height: 30px; border: none; background: none; cursor: pointer;
  border-radius: 8px; display: grid; place-items: center; color: var(--ink-soft);
  transition: background .15s, color .15s;
}
.modal-hint {
  margin: 0 0 8px; padding: 8px 12px; border-radius: 8px;
  background: rgba(59,63,224,.06); color: var(--brand-ink);
  font: 500 12px/1.4 'Inter', sans-serif;
}
.modal-close:hover { background: var(--bg); color: var(--ink); }
.modal-close svg { width: 16px; height: 16px; }
.modal-body { padding: 8px 20px 20px; overflow-y: auto; }
.modal-state { padding: 32px 0; text-align: center; color: var(--ink-soft); font: 500 13px/1 'Inter', sans-serif; }

.hist-list { list-style: none; margin: 0; padding: 0; }
.hist-item { padding: 14px 0; border-bottom: 1px solid var(--line); }
.hist-item:last-child { border-bottom: none; }
.hist-item-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.hist-action {
  font: 600 13px/1.3 'Inter', sans-serif; color: var(--ink);
  background: rgba(59,63,224,.08); padding: 4px 10px; border-radius: 7px;
}
.hist-time { font: 12px/1.3 'Inter', sans-serif; color: var(--ink-soft); white-space: nowrap; flex-shrink: 0; margin-top: 2px; }
.hist-fields { margin-top: 10px; display: flex; flex-direction: column; gap: 6px; }
.hist-field { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font: 13px/1.4 'Inter', sans-serif; }
.hist-field-label { color: var(--ink-soft); }
.hist-val { color: var(--ink); }
.hist-val.old { color: #b45309; text-decoration: line-through; opacity: .8; }
.hist-val.new { color: #065f46; font-weight: 600; }
.hist-arrow { width: 14px; height: 14px; color: var(--ink-soft); flex-shrink: 0; }
.hist-item-foot { margin-top: 10px; }
.hist-restore {
  height: 30px; padding: 0 14px; border-radius: 8px; cursor: pointer;
  border: 1.5px solid var(--brand); background: rgba(59,63,224,.06); color: var(--brand);
  font: 600 12px/1 'Inter', sans-serif;
  transition: background .15s, opacity .15s;
}
.hist-restore:hover:not(:disabled) { background: rgba(59,63,224,.12); }
.hist-restore:disabled { opacity: .5; cursor: default; }
.hist-restored-badge {
  display: inline-flex; align-items: center; gap: 6px;
  height: 30px; padding: 0 12px; border-radius: 8px;
  border: 1.5px solid var(--border, #d9dce3); background: rgba(120,125,140,.08);
  color: var(--muted, #7a8090); font: 600 12px/1 'Inter', sans-serif;
}
.hist-restored-badge svg { width: 14px; height: 14px; }

.modal-enter-active, .modal-leave-active { transition: opacity .2s var(--ease); }
.modal-enter-from, .modal-leave-to { opacity: 0; }
.modal-enter-active .modal-card, .modal-leave-active .modal-card { transition: transform .2s var(--ease); }
.modal-enter-from .modal-card, .modal-leave-to .modal-card { transform: translateY(12px); }

/* ── Действия в строке + модалка правки ── */
.row-actions { display: flex; gap: 8px; }
.modal-card--narrow { max-width: 440px; }
.edit-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.edit-field { display: flex; flex-direction: column; gap: 6px; font: 500 12px/1 'Inter', sans-serif; color: var(--ink-soft); }
.edit-input {
  height: 40px; box-sizing: border-box; border: 1.5px solid var(--line); border-radius: 8px;
  padding: 0 12px; font: 13px/1 'Inter', sans-serif; color: var(--ink); background: #fff;
  outline: none; transition: border-color .15s, box-shadow .15s;
}
.edit-input:focus { border-color: var(--brand); box-shadow: 0 0 0 3px rgba(59,63,224,.1); }
.edit-foot { display: flex; justify-content: flex-end; gap: 10px; padding: 14px 20px; border-top: 1px solid var(--line); }
.edit-btn-cancel {
  height: 40px; padding: 0 16px; border: 1.5px solid var(--line); border-radius: 8px;
  background: var(--card); color: var(--ink-soft); font: 600 13px/1 'Inter', sans-serif; cursor: pointer;
  transition: border-color .15s, color .15s;
}
.edit-btn-cancel:hover { border-color: var(--brand); color: var(--brand); }
.edit-btn-save {
  height: 40px; padding: 0 20px; border: 1.5px solid var(--brand); border-radius: 8px;
  background: var(--brand); color: #fff; font: 600 13px/1 'Inter', sans-serif; cursor: pointer;
  transition: background .15s;
}
.edit-btn-save:hover:not(:disabled) { background: var(--brand-ink); }
.edit-btn-save:disabled { opacity: .5; cursor: not-allowed; }

/* ── Responsive ── */
@media (max-width: 600px) {
  .page-bar, .filters-bar, .results-bar, .pagination { padding-left: 16px; padding-right: 16px; }
  .table-card { margin-left: 16px; margin-right: 16px; }
  .search-wrap { max-width: 100%; }
  .edit-grid { grid-template-columns: 1fr; }
}
</style>
