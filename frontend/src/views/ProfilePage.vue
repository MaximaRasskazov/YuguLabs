<script setup>
import { ref, computed, reactive, watch } from 'vue'
import { useAuthStore } from '../stores/auth'
import { authApi } from '../api/auth'
import { saveBlob, filenameFromDisposition } from '../utils/download'
import AppHeader from '../components/AppHeader.vue'
import AppSidebar from '../components/AppSidebar.vue'

const auth = useAuthStore()
const sidebarOpen = ref(false)

const ROLE_LABEL = { STUDENT: 'Студент', TEACHER: 'Преподаватель', DEAN: 'Деканат' }

// ── Личные данные (ФИО / группа) ──────────────────────────
// Редактирование профиля: PATCH /api/me. Изменения логируются на бэкенде
// в change_logs (видны в админском журнале изменений).
const edit = reactive({ lastName: '', firstName: '', middleName: '', group: '' })
const editError = ref('')
const editSuccess = ref(false)
const editLoading = ref(false)

function resetEdit() {
  edit.lastName   = auth.user?.lastName   ?? ''
  edit.firstName  = auth.user?.firstName  ?? ''
  edit.middleName = auth.user?.middleName ?? ''
  edit.group      = auth.user?.group      ?? ''
}
resetEdit()
watch(() => auth.user, resetEdit)

const editDirty = computed(() =>
  edit.lastName   !== (auth.user?.lastName   ?? '') ||
  edit.firstName  !== (auth.user?.firstName  ?? '') ||
  edit.middleName !== (auth.user?.middleName ?? '') ||
  (auth.isStudent && edit.group !== (auth.user?.group ?? '')),
)

async function saveProfile() {
  editError.value = ''
  editSuccess.value = false
  if (!edit.firstName.trim()) { editError.value = 'Имя не может быть пустым'; return }
  if (!edit.lastName.trim())  { editError.value = 'Фамилия не может быть пустой'; return }
  editLoading.value = true
  try {
    const payload = {
      first_name:  edit.firstName.trim(),
      last_name:   edit.lastName.trim(),
      middle_name: edit.middleName.trim(),
    }
    if (auth.isStudent) payload.group_name = edit.group.trim()
    const { data } = await authApi.updateProfile(payload)
    auth.setProfile(data.user)
    editSuccess.value = true
    setTimeout(() => { editSuccess.value = false }, 3000)
  } catch (e) {
    editError.value = e.response?.data?.message || 'Не удалось сохранить данные'
  } finally {
    editLoading.value = false
  }
}

// ── Password ──────────────────────────────────────────────
const pw = reactive({ current: '', next: '', confirm: '' })
const pwError = ref('')
const pwSuccess = ref(false)
const pwLoading = ref(false)

async function changePassword() {
  pwError.value = ''
  pwSuccess.value = false
  if (!pw.current)               { pwError.value = 'Введите текущий пароль'; return }
  if (pw.next.length < 8)        { pwError.value = 'Минимум 8 символов'; return }
  if (pw.next !== pw.confirm)    { pwError.value = 'Пароли не совпадают'; return }
  pwLoading.value = true
  try {
    await authApi.changePassword(pw.current, pw.next)
    pw.current = ''; pw.next = ''; pw.confirm = ''
    pwSuccess.value = true
    setTimeout(() => { pwSuccess.value = false }, 3000)
  } catch (e) {
    const msg = e.response?.data?.message || e.response?.data?.error
    pwError.value = msg || 'Неверный текущий пароль'
  } finally {
    pwLoading.value = false
  }
}

// ── Avatar ────────────────────────────────────────────────
const fileInput = ref(null)
const avatarLoading = ref(false)
const avatarError = ref('')

// Бэкенд сам сжимает и уменьшает большие снимки (вплоть до 4K), поэтому
// клиентский лимит щедрый — 16 МБ, как и серверный MaxUploadBytes.
const MAX_AVATAR = 16 * 1024 * 1024
const ALLOWED_AVATAR = ['image/jpeg', 'image/png', 'image/webp']
const avatarDownloading = ref(false)

function pickAvatar() {
  avatarError.value = ''
  fileInput.value?.click()
}

async function onAvatarPicked(e) {
  const file = e.target.files?.[0]
  e.target.value = '' // сброс — чтобы повторный выбор того же файла сработал
  if (!file) return
  if (!ALLOWED_AVATAR.includes(file.type)) { avatarError.value = 'Нужен JPEG, PNG или WebP'; return }
  if (file.size > MAX_AVATAR) { avatarError.value = 'Файл больше 16 МБ'; return }
  avatarLoading.value = true
  try {
    const { data } = await authApi.uploadAvatar(file)
    auth.setAvatar(data.avatar_url)
  } catch (err) {
    avatarError.value = err.response?.data?.message || 'Не удалось загрузить фото'
  } finally {
    avatarLoading.value = false
  }
}

async function removeAvatar() {
  avatarError.value = ''
  avatarLoading.value = true
  try {
    await authApi.deleteAvatar()
    auth.setAvatar(null)
  } catch {
    avatarError.value = 'Не удалось удалить фото'
  } finally {
    avatarLoading.value = false
  }
}

// Скачивает сжатый оригинал — он недоступен по прямому URL, только через
// защищённый маршрут с проверкой владельца.
async function downloadOriginal() {
  avatarError.value = ''
  avatarDownloading.value = true
  try {
    const resp = await authApi.downloadOriginal()
    const name = filenameFromDisposition(resp.headers['content-disposition'], 'photo.jpg')
    saveBlob(resp.data, name)
  } catch {
    avatarError.value = 'Не удалось скачать оригинал'
  } finally {
    avatarDownloading.value = false
  }
}

// ── Computed ──────────────────────────────────────────────
const initials = computed(() => {
  const f = auth.user?.firstName?.[0] ?? ''
  const l = auth.user?.lastName?.[0] ?? ''
  return (f + l).toUpperCase()
})

const fullName = computed(() =>
  [auth.user?.lastName, auth.user?.firstName, auth.user?.middleName].filter(Boolean).join(' ')
)
</script>

<template>
  <div class="profile-root">

    <AppSidebar :open="sidebarOpen" @close="sidebarOpen = false" />

    <div class="page-wrap">
      <AppHeader @open-sidebar="sidebarOpen = true" />

      <main class="main">
        <div class="profile-card">

          <!-- Avatar + Name -->
          <div class="hero-section">
            <button type="button" class="avatar-btn" :disabled="avatarLoading" title="Сменить фото" @click="pickAvatar">
              <img
                v-if="auth.user?.avatar"
                :src="auth.user.avatar"
                alt="Аватар"
                class="avatar-img"
                @error="auth.setAvatar(null)"
              />
              <span v-else class="avatar-initials">{{ initials }}</span>
              <span class="avatar-overlay">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/>
                  <circle cx="12" cy="13" r="4"/>
                </svg>
              </span>
            </button>
            <input
              ref="fileInput"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              class="file-hidden"
              @change="onAvatarPicked"
            />
            <div class="hero-meta">
              <div class="hero-name">{{ fullName }}</div>
              <span class="role-label">{{ ROLE_LABEL[auth.role] ?? auth.role }}</span>
              <div class="avatar-actions">
                <button
                  v-if="auth.user?.avatar"
                  type="button"
                  class="avatar-remove"
                  :disabled="avatarLoading"
                  @click="removeAvatar"
                >Удалить фото</button>
                <button
                  v-if="auth.user?.avatar"
                  type="button"
                  class="avatar-link"
                  :disabled="avatarDownloading"
                  @click="downloadOriginal"
                >{{ avatarDownloading ? 'Скачивание…' : 'Скачать оригинал' }}</button>
                <span v-if="avatarLoading" class="avatar-hint">Загрузка…</span>
                <span v-if="avatarError" class="avatar-err">{{ avatarError }}</span>
              </div>
            </div>
          </div>

          <div class="section-divider" />

          <!-- ── Личные данные (редактируемые) ── -->
          <div class="section-title">Личные данные</div>
          <div class="fields-grid">
            <div class="field-group">
              <label class="field-label">Фамилия</label>
              <input class="field-input" v-model="edit.lastName" placeholder="Фамилия" />
            </div>
            <div class="field-group">
              <label class="field-label">Имя</label>
              <input class="field-input" v-model="edit.firstName" placeholder="Имя" />
            </div>
            <div class="field-group">
              <label class="field-label">Отчество</label>
              <input class="field-input" v-model="edit.middleName" placeholder="—" />
            </div>
            <div v-if="auth.isStudent" class="field-group">
              <label class="field-label">Группа</label>
              <input class="field-input" v-model="edit.group" placeholder="Напр. ИВТ-21" />
            </div>
            <div v-else class="field-group">
              <label class="field-label">Должность</label>
              <input class="field-input field-readonly" :value="ROLE_LABEL[auth.role]" readonly />
            </div>
          </div>
          <div class="edit-actions">
            <button class="btn-primary" :disabled="editLoading || !editDirty" @click="saveProfile">
              {{ editLoading ? 'Сохранение…' : 'Сохранить' }}
            </button>
            <span v-if="editSuccess" class="pw-success edit-status">Данные обновлены</span>
            <span v-if="editError" class="pw-error edit-status">{{ editError }}</span>
          </div>

          <!-- ── Email ── -->
          <div class="section-title">Почта</div>
          <div class="email-row">
            <div class="email-icon-wrap">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                <rect x="2" y="4" width="20" height="16" rx="2"/>
                <path d="M2 7l10 7 10-7"/>
              </svg>
            </div>
            <span class="email-text">{{ auth.user?.email }}</span>
          </div>

          <div class="section-divider" />

          <!-- ── Password ── -->
          <div class="section-title">Изменить пароль</div>
          <div class="pw-row">
            <div class="field-group">
              <label class="field-label">Текущий пароль</label>
              <input class="field-input" type="password" v-model="pw.current" placeholder="••••••" />
            </div>
            <div class="field-group">
              <label class="field-label">Новый пароль</label>
              <input class="field-input" type="password" v-model="pw.next" placeholder="••••••" />
            </div>
            <div class="field-group">
              <label class="field-label">Повтор пароля</label>
              <input class="field-input" type="password" v-model="pw.confirm" placeholder="••••••" />
            </div>
            <div class="pw-action">
              <label class="field-label">&nbsp;</label>
              <button class="btn-outline" :disabled="pwLoading" @click="changePassword">
                {{ pwLoading ? 'Сохранение…' : 'Изменить' }}
              </button>
            </div>
          </div>
          <p v-if="pwError" class="pw-error">{{ pwError }}</p>
          <p v-if="pwSuccess" class="pw-success">Пароль успешно изменён</p>


        </div>
      </main>
    </div>
  </div>
</template>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');

*, *::before, *::after { box-sizing: border-box; }

.profile-root {
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

.page-wrap {
  min-height: 100dvh;
  background: var(--bg);
  display: flex;
  flex-direction: column;
}

.main {
  flex: 1;
  padding: 32px 24px;
  display: flex;
  justify-content: center;
  align-items: flex-start;
}

/* ── Profile card ── */
.profile-card {
  width: 100%;
  max-width: 740px;
  background: var(--card);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  border: 1px solid var(--line);
  padding: 32px;
}

/* ── Hero ── */
.hero-section {
  display: flex;
  align-items: center;
  gap: 20px;
  margin-bottom: 28px;
}

.avatar-btn {
  position: relative;
  width: 88px; height: 88px; border-radius: 50%; flex-shrink: 0;
  background: linear-gradient(135deg, #2b5cff, #8b3df0);
  border: none; cursor: pointer; overflow: hidden; padding: 0;
  display: grid; place-items: center;
}
.avatar-img { width: 100%; height: 100%; object-fit: cover; display: block; }
.avatar-initials {
  font: 700 28px/1 'Inter', sans-serif; color: #fff;
  pointer-events: none;
}
.avatar-overlay {
  position: absolute; inset: 0;
  background: rgba(10,12,30,.45);
  display: grid; place-items: center;
  opacity: 0; transition: opacity .2s var(--ease);
}
.avatar-btn:hover .avatar-overlay { opacity: 1; }
.avatar-btn:disabled { cursor: default; opacity: .7; }
.avatar-overlay svg { width: 24px; height: 24px; stroke: #fff; }

.file-hidden { display: none; }

.avatar-actions { display: flex; align-items: center; gap: 10px; margin-top: 2px; min-height: 16px; }
.avatar-remove {
  background: none; border: none; padding: 0; cursor: pointer;
  font: 500 12px/1 'Inter', sans-serif; color: var(--ink-soft);
  text-decoration: underline; transition: color .15s;
}
.avatar-remove:hover { color: #dc2626; }
.avatar-remove:disabled { opacity: .5; cursor: default; }
.avatar-hint { font: 12px/1 'Inter', sans-serif; color: var(--ink-soft); }
.avatar-err  { font: 12px/1 'Inter', sans-serif; color: #dc2626; }

.hero-meta { display: flex; flex-direction: column; gap: 6px; }
.hero-name {
  font: 700 20px/1.2 'Inter', sans-serif;
  color: var(--ink);
}

.role-label {
  font: 500 13px/1 'Inter', sans-serif;
  color: var(--ink-soft);
}

/* ── Divider ── */
.section-divider {
  border: none; border-top: 1px solid var(--line); margin: 0 0 24px;
}

/* ── Fields grid ── */
.fields-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 24px;
  margin-bottom: 28px;
}

.field-group { display: flex; flex-direction: column; gap: 6px; }
.field-label {
  font: 500 12px/1 'Inter', sans-serif;
  color: var(--ink-soft);
  text-transform: none;
}

.field-input {
  appearance: none;
  height: 44px; width: 100%;
  border: 1.5px solid var(--line);
  border-radius: var(--radius);
  background: #fff;
  padding: 0 14px;
  font: 14px/1 'Inter', sans-serif;
  color: var(--ink);
  outline: none;
  transition: border-color .2s var(--ease), box-shadow .2s var(--ease);
}
.field-input:focus { border-color: var(--brand); box-shadow: 0 0 0 4px rgba(59,63,224,.1); }
.field-input::placeholder { color: #b7b9c2; }

.field-readonly {
  background: #f3f4f7;
  color: var(--ink-soft);
  cursor: default;
}
.field-readonly:focus { border-color: var(--line); box-shadow: none; }

/* ── Section title ── */
.section-title {
  font: 700 14px/1 'Inter', sans-serif;
  color: var(--ink);
  margin-bottom: 14px;
}

/* ── Email ── */
.email-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 24px;
  flex-wrap: wrap;
}

.email-icon-wrap {
  width: 36px; height: 36px; border-radius: 10px;
  background: linear-gradient(135deg, #2b5cff, #8b3df0);
  display: grid; place-items: center; flex-shrink: 0;
}
.email-icon-wrap svg { width: 16px; height: 16px; stroke: #fff; }

.email-text {
  font: 14px/1 'Inter', sans-serif;
  color: var(--ink);
  flex: 1;
}

/* ── Password ── */
.pw-row {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr auto;
  gap: 16px;
  align-items: end;
  margin-bottom: 8px;
}

.pw-action { display: flex; flex-direction: column; gap: 6px; }

.btn-outline {
  height: 44px; padding: 0 18px;
  background: none; border: 1.5px solid var(--line); border-radius: var(--radius);
  font: 500 13px/1 'Inter', sans-serif; color: var(--ink-soft);
  cursor: pointer; white-space: nowrap;
  transition: border-color .15s, color .15s, background .15s;
}
.btn-outline:hover { border-color: var(--brand); color: var(--brand); background: rgba(59,63,224,.04); }

/* ── Кнопка сохранения профиля ── */
.btn-primary {
  height: 44px; padding: 0 22px;
  border: 1.5px solid var(--brand); border-radius: var(--radius);
  background: var(--brand); color: #fff;
  font: 600 13px/1 'Inter', sans-serif; cursor: pointer; white-space: nowrap;
  transition: background .15s, border-color .15s, opacity .15s;
}
.btn-primary:hover:not(:disabled) { background: var(--brand-ink); border-color: var(--brand-ink); }
.btn-primary:disabled { opacity: .5; cursor: not-allowed; }

.edit-actions { display: flex; align-items: center; gap: 14px; margin-bottom: 8px; }
.edit-status { margin: 0; }

/* ── Ссылка «Скачать оригинал» ── */
.avatar-link {
  background: none; border: none; padding: 0; cursor: pointer;
  font: 500 12px/1 'Inter', sans-serif; color: var(--brand);
  text-decoration: underline; transition: color .15s;
}
.avatar-link:hover:not(:disabled) { color: var(--brand-ink); }
.avatar-link:disabled { opacity: .5; cursor: default; }

.pw-error {
  font: 13px/1 'Inter', sans-serif;
  color: #dc2626;
  margin: 0 0 16px;
}
.pw-success {
  font: 13px/1 'Inter', sans-serif;
  color: #059669;
  margin: 0 0 16px;
}

/* ── Responsive ── */
@media (max-width: 640px) {
  .main { padding: 16px; }
  .profile-card { padding: 20px; }
  .fields-grid { grid-template-columns: 1fr; }
  .pw-row { grid-template-columns: 1fr; }
  .pw-action { display: none; }
  .hero-name { font-size: 17px; }
}
</style>
