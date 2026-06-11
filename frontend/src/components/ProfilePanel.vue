<script setup>
import { ref, computed, reactive, watch } from 'vue'
import { useAuthStore } from '../stores/auth'
import { authApi } from '../api/auth'
import { teacherRequestsApi } from '../api/teacherRequests'

const props = defineProps({ open: Boolean })
defineEmits(['close'])

const auth = useAuthStore()

const ROLE_LABEL = { STUDENT: 'Студент', TEACHER: 'Преподаватель', DEAN: 'Деканат' }

// ── Повышение до преподавателя (только для STUDENT) ───────
const isStudent = computed(() => auth.role === 'STUDENT')

// Статус последней заявки текущего пользователя: '' | 'pending' | 'approved' | 'rejected'.
const promoStatus  = ref('')
const promoLoading = ref(false)

const promoModal   = ref(false)
const promoReason  = ref('')
const promoError   = ref('')
const promoSubmitting = ref(false)
const promoDone    = ref(false)

// Загружаем историю заявок при открытии панели, чтобы знать, есть ли
// уже заявка на рассмотрении и не плодить дубли.
async function loadPromoStatus() {
  if (!isStudent.value) return
  promoLoading.value = true
  try {
    const list = await teacherRequestsApi.listMy()
    const items = Array.isArray(list) ? list : (list?.items ?? [])
    // Берём самую свежую заявку (сортировка по created_at desc).
    const latest = items
      .slice()
      .sort((a, b) => new Date(b.created_at) - new Date(a.created_at))[0]
    promoStatus.value = latest?.status ?? ''
  } catch {
    promoStatus.value = '' // не критично — просто покажем кнопку
  } finally {
    promoLoading.value = false
  }
}

watch(() => props.open, (open) => {
  if (open) {
    promoDone.value = false
    loadPromoStatus()
  }
}, { immediate: true })

function openPromo() {
  promoReason.value = ''
  promoError.value = ''
  promoModal.value = true
}
function closePromo() { promoModal.value = false }

async function submitPromo() {
  const reason = promoReason.value.trim()
  if (!reason) { promoError.value = 'Заполните описание'; return }
  promoSubmitting.value = true
  promoError.value = ''
  try {
    await teacherRequestsApi.create(reason)
    promoStatus.value = 'pending'
    promoModal.value = false
    promoDone.value = true
  } catch (e) {
    const code = e.response?.data?.error
    if (code === 'already_pending') {
      promoStatus.value = 'pending'
      promoModal.value = false
    } else {
      promoError.value = e.response?.data?.message || 'Не удалось отправить заявку'
    }
  } finally {
    promoSubmitting.value = false
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
  if (!pw.current)             { pwError.value = 'Введите текущий пароль'; return }
  if (pw.next.length < 8)      { pwError.value = 'Минимум 8 символов'; return }
  if (pw.next !== pw.confirm)  { pwError.value = 'Пароли не совпадают'; return }
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

// ── Computed ──────────────────────────────────────────────
const initials = computed(() => {
  const f = auth.user?.firstName?.[0] ?? ''
  const l = auth.user?.lastName?.[0] ?? ''
  return (f + l).toUpperCase()
})

const fullName = computed(() =>
  [auth.user?.lastName, auth.user?.firstName, auth.user?.middleName].filter(Boolean).join(' ')
)
// ── Аватар ────────────────────────────────────────────────
const fileInput = ref(null)
const avatarLoading = ref(false)
const avatarError = ref('')
const MAX_AVATAR = 2 * 1024 * 1024 // 2 МБ — синхронно с бэкендом
const ALLOWED_AVATAR = ['image/jpeg', 'image/png', 'image/webp']

function pickAvatar() {
  avatarError.value = ''
  fileInput.value?.click()
}
async function onAvatarPicked(e) {
  const file = e.target.files?.[0]
  e.target.value = '' // сброс — чтобы повторный выбор того же файла сработал
  if (!file) return
  if (!ALLOWED_AVATAR.includes(file.type)) { avatarError.value = 'Нужен JPEG, PNG или WebP'; return }
  if (file.size > MAX_AVATAR) { avatarError.value = 'Файл больше 2 МБ'; return }
  avatarLoading.value = true
  try {
    const { data } = await authApi.uploadAvatar(file)
    auth.setAvatar(data.avatar_url)
  } catch {
    avatarError.value = 'Не удалось загрузить фото'
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
</script>

<template>
  <Teleport to="body">
    <Transition name="panel">
      <div v-if="open" class="panel-root">

        <!-- Overlay -->
        <div class="panel-overlay" @click="$emit('close')" />

        <!-- Panel -->
        <div class="panel">

          <!-- Head -->
          <div class="panel-head">
            <span class="panel-title">Профиль</span>
            <button class="panel-close" @click="$emit('close')" aria-label="Закрыть">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>

          <!-- Scrollable body -->
          <div class="panel-body">

            <!-- Hero -->
            <div class="hero">
              <button
                type="button"
                class="avatar-display"
                :disabled="avatarLoading"
                title="Сменить фото"
                @click="pickAvatar"
              >
                <img
                  v-if="auth.user?.avatar"
                  :src="auth.user.avatar"
                  alt="Аватар"
                  class="avatar-img"
                  @error="auth.setAvatar(null)"
                />
                <span v-else class="avatar-initials">{{ initials }}</span>
                <span class="avatar-cam">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/><circle cx="12" cy="13" r="4"/>
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
              <div class="hero-name">{{ fullName }}</div>
              <span class="role-chip">{{ ROLE_LABEL[auth.role] }}</span>
              <button v-if="auth.user?.avatar" type="button" class="avatar-remove" :disabled="avatarLoading" @click="removeAvatar">
                Удалить фото
              </button>
              <span v-if="avatarError" class="avatar-err">{{ avatarError }}</span>

              <!-- Повышение до преподавателя (только для студента) -->
              <template v-if="isStudent">
                <button
                  v-if="promoStatus === 'pending'"
                  class="promo-chip promo-chip--pending"
                  type="button" disabled
                >
                  Заявка на рассмотрении
                </button>
                <button
                  v-else-if="promoStatus === 'rejected'"
                  class="promo-link"
                  type="button"
                  @click="openPromo"
                >
                  Заявка отклонена — подать снова
                </button>
                <button
                  v-else-if="promoStatus !== 'approved'"
                  class="promo-link"
                  type="button"
                  :disabled="promoLoading"
                  @click="openPromo"
                >
                  Повысить до преподавателя
                </button>
              </template>
            </div>

            <div class="divider" />

            <!-- Email -->
            <div class="section-label">Почта</div>

            <div class="email-row">
              <span class="email-text">{{ auth.user?.email }}</span>
            </div>

            <div class="divider" />

            <!-- Password -->
            <div class="section-label">Изменить пароль</div>

            <div class="field-group">
              <label class="field-label">Текущий пароль</label>
              <input class="field-input" type="password" v-model="pw.current" placeholder="••••••" />
            </div>
            <div class="field-group">
              <label class="field-label">Новый пароль</label>
              <input class="field-input" type="password" v-model="pw.next" placeholder="••••••" />
            </div>
            <div class="field-group">
              <label class="field-label">Повторите пароль</label>
              <input class="field-input" type="password" v-model="pw.confirm" placeholder="••••••" />
            </div>

            <p v-if="pwError" class="pw-error">{{ pwError }}</p>
            <p v-if="pwSuccess" class="pw-success">Пароль успешно изменён</p>

            <button class="btn-outline btn-full" :disabled="pwLoading" @click="changePassword">
              {{ pwLoading ? 'Сохранение…' : 'Изменить пароль' }}
            </button>

          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- ── Модалка повышения до преподавателя ── -->
  <Teleport to="body">
    <Transition name="promo-modal">
      <div v-if="promoModal" class="promo-overlay" @click.self="closePromo">
        <div class="promo-modal">

          <div class="promo-head">
            <span class="promo-title">Повышение до преподавателя</span>
            <button class="panel-close" @click="closePromo" aria-label="Закрыть">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>

          <div class="promo-body">
            <p class="promo-desc">
              Вы можете подать заявку на смену роли со «Студента» на «Преподавателя».
              Заявку рассмотрит деканат: после одобрения вам станут доступны функции
              преподавателя. Опишите, почему вам нужна эта роль.
            </p>

            <div class="field-group">
              <label class="field-label">Описание <span class="required">*</span></label>
              <textarea
                class="promo-textarea"
                v-model="promoReason"
                rows="4"
                placeholder="Например: веду семинары по дисциплине и хочу принимать пересдачи…"
              />
            </div>

            <p v-if="promoError" class="pw-error">{{ promoError }}</p>
          </div>

          <div class="promo-foot">
            <button class="btn-outline" @click="closePromo">Отмена</button>
            <button class="btn-primary" :disabled="promoSubmitting" @click="submitPromo">
              {{ promoSubmitting ? 'Отправка…' : 'Отправить' }}
            </button>
          </div>

        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* ── Variables (inherited from parent or set here) ── */
.panel-root {
  --card:      #ffffff;
  --ink:       #1a1d24;
  --ink-soft:  #6b7280;
  --line:      #d7d9e0;
  --bg:        #f3f4f7;
  --brand:     #3b3fe0;
  --brand-ink: #2a2e9e;
  --radius:    10px;
  --ease:      cubic-bezier(.2,.7,.2,1);
  font-family: 'Inter', system-ui, sans-serif;
}

/* ── Layout ── */
.panel-root {
  position: fixed; inset: 0; z-index: 300;
  display: flex; justify-content: flex-end;
}

.panel-overlay {
  position: absolute; inset: 0;
  background: rgba(10,12,30,.4);
  backdrop-filter: blur(2px);
  -webkit-backdrop-filter: blur(2px);
}

.panel {
  position: relative; z-index: 1;
  width: 380px; max-width: 100vw;
  height: 100%;
  background: var(--card);
  box-shadow: -8px 0 40px rgba(20,22,60,.16);
  display: flex; flex-direction: column;
  border-radius: 16px 0 0 16px;
  overflow: hidden;
}

/* ── Head ── */
.panel-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 20px 20px 16px;
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}
.panel-title {
  font-family: 'Gerhaus', 'Inter', sans-serif;
  font-size: 15px; font-weight: 700; color: #3C38B6;
}
.panel-close {
  width: 32px; height: 32px; border-radius: 8px;
  background: none; border: none; cursor: pointer;
  display: grid; place-items: center; color: var(--ink-soft);
  transition: background .15s, color .15s;
}
.panel-close:hover { background: var(--bg); color: var(--ink); }
.panel-close svg { width: 16px; height: 16px; }

/* ── Body ── */
.panel-body {
  flex: 1; overflow-y: auto; overflow-x: hidden; padding: 24px 20px;
  display: flex; flex-direction: column; gap: 12px;
}
.panel-body::-webkit-scrollbar { width: 4px; }
.panel-body::-webkit-scrollbar-track { background: transparent; }
.panel-body::-webkit-scrollbar-thumb { background: var(--line); border-radius: 4px; }

/* ── Hero ── */
.hero {
  display: flex; flex-direction: column; align-items: center;
  gap: 8px; padding-bottom: 4px;
}

.avatar-display {
  position: relative;
  width: 88px; height: 88px; border-radius: 50%; flex-shrink: 0;
  background: linear-gradient(135deg, #2b5cff, #8b3df0);
  display: grid; place-items: center;
  overflow: hidden;
  border: none; padding: 0; cursor: pointer;
}
.avatar-display:disabled { cursor: default; opacity: .7; }
.avatar-display .avatar-img { width: 100%; height: 100%; object-fit: cover; display: block; }
.avatar-cam {
  position: absolute; inset: 0;
  background: rgba(10,12,30,.45);
  display: grid; place-items: center;
  opacity: 0; transition: opacity .18s ease;
}
.avatar-display:hover .avatar-cam { opacity: 1; }
.avatar-cam svg { width: 22px; height: 22px; stroke: #fff; }
.file-hidden { display: none; }
.avatar-remove {
  margin-top: 2px; background: none; border: none; padding: 0; cursor: pointer;
  font: 500 12px/1 'Inter', sans-serif; color: #6b7280; text-decoration: underline;
  transition: color .15s;
}
.avatar-remove:hover { color: #dc2626; }
.avatar-remove:disabled { opacity: .5; cursor: default; }
.avatar-err { font: 12px/1 'Inter', sans-serif; color: #dc2626; }
.avatar-initials {
  font: 700 28px/1 'Inter', sans-serif; color: #fff; pointer-events: none;
}

.hero-name {
  font: 700 16px/1.3 'Inter', sans-serif;
  color: var(--ink); text-align: center;
}

.role-chip {
  padding: 3px 12px; border-radius: 20px;
  background: rgba(59,63,224,.1); color: var(--brand-ink);
  font: 600 12px/1.4 'Inter', sans-serif;
}


/* ── Divider ── */
.divider { border: none; border-top: 1px solid var(--line); margin: 4px 0; }

/* ── Section label ── */
.section-label {
  font: 600 12px/1 'Inter', sans-serif;
  color: var(--ink-soft);
  text-transform: uppercase;
  letter-spacing: .05em;
  margin-bottom: 2px;
}

/* ── Fields ── */
.field-group { display: flex; flex-direction: column; gap: 5px; }
.field-label { font: 500 12px/1 'Inter', sans-serif; color: var(--ink-soft); }

.field-input {
  appearance: none; height: 40px; width: 100%; box-sizing: border-box;
  border: 1.5px solid var(--line); border-radius: var(--radius);
  background: #fff; padding: 0 12px;
  font: 13px/1 'Inter', sans-serif; color: var(--ink); outline: none;
  transition: border-color .2s var(--ease), box-shadow .2s var(--ease);
}
.field-input:focus { border-color: var(--brand); box-shadow: 0 0 0 3px rgba(59,63,224,.1); }
.field-input::placeholder { color: #b7b9c2; }

/* ── Email row ── */
.email-row {
  display: flex; align-items: center; gap: 10px; min-width: 0;
}
.email-text {
  flex: 1; font: 13px/1 'Inter', sans-serif; color: var(--ink);
  min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}

/* ── Password ── */
.pw-error  { font: 12px/1.4 'Inter', sans-serif; color: #dc2626; margin: 0; }
.pw-success { font: 12px/1.4 'Inter', sans-serif; color: #059669; margin: 0; }

/* ── Buttons ── */
.btn-full { width: 100%; box-sizing: border-box; }

.btn-outline {
  height: 44px; padding: 0 18px;
  background: #fff;
  border: 1.5px solid #c5c7d4; border-radius: var(--radius);
  font: 600 13px/1 'Inter', sans-serif; color: var(--ink);
  cursor: pointer; white-space: nowrap;
  box-shadow: 0 1px 4px rgba(20,22,60,.08);
  transition: border-color .15s, color .15s, background .15s, box-shadow .15s;
}
.btn-outline:hover {
  border-color: var(--brand); color: var(--brand);
  background: rgba(59,63,224,.04);
  box-shadow: 0 2px 10px rgba(59,63,224,.14);
}


/* ── Повышение до преподавателя ── */
.promo-link {
  margin-top: 6px;
  background: none; border: none; padding: 2px 4px;
  font: 600 13px/1.3 'Inter', sans-serif; color: var(--brand);
  cursor: pointer; text-decoration: underline; text-underline-offset: 2px;
  transition: color .15s;
}
.promo-link:hover:not(:disabled) { color: var(--brand-ink); }
.promo-link:disabled { opacity: .5; cursor: default; }

.promo-chip {
  margin-top: 6px; padding: 4px 12px; border-radius: 20px; border: none;
  font: 600 12px/1.4 'Inter', sans-serif;
}
.promo-chip--pending {
  background: rgba(245,158,11,.15); color: #b45309; cursor: default;
}

/* ── Promo modal ── */
.promo-overlay {
  position: fixed; inset: 0; z-index: 410;
  background: rgba(10,12,30,.5);
  backdrop-filter: blur(3px); -webkit-backdrop-filter: blur(3px);
  display: flex; align-items: center; justify-content: center; padding: 20px;
  --card: #fff; --ink: #1a1d24; --ink-soft: #6b7280; --line: #d7d9e0;
  --bg: #f3f4f7; --brand: #3b3fe0; --brand-ink: #2a2e9e;
  --radius: 10px; --ease: cubic-bezier(.2,.7,.2,1);
  font-family: 'Inter', system-ui, sans-serif;
}
.promo-modal {
  background: var(--card); border-radius: 16px; width: 100%; max-width: 440px;
  box-shadow: 0 24px 64px -12px rgba(10,12,30,.3);
  display: flex; flex-direction: column; max-height: 90vh; overflow: hidden;
}
.promo-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 20px; border-bottom: 1px solid var(--line);
}
.promo-title {
  font-family: 'Gerhaus', 'Inter', sans-serif;
  font-size: 15px; font-weight: 600; color: #3C38B6;
}
.promo-body {
  padding: 20px; display: flex; flex-direction: column; gap: 14px;
  overflow-y: auto;
}
.promo-desc {
  margin: 0; font: 13px/1.6 'Inter', sans-serif; color: var(--ink-soft);
}
.required { color: #dc2626; }
.promo-textarea {
  width: 100%; box-sizing: border-box; padding: 10px 12px; resize: vertical;
  border: 1.5px solid var(--line); border-radius: var(--radius);
  background: #fff; font: 13px/1.5 'Inter', sans-serif; color: var(--ink);
  outline: none; transition: border-color .2s var(--ease), box-shadow .2s var(--ease);
}
.promo-textarea:focus { border-color: var(--brand); box-shadow: 0 0 0 3px rgba(59,63,224,.1); }
.promo-foot {
  display: flex; justify-content: flex-end; gap: 10px;
  padding: 14px 20px; border-top: 1px solid var(--line);
}
.btn-primary {
  padding: 0 20px; height: 40px;
  border: 1.5px solid var(--brand); border-radius: var(--radius);
  background: var(--brand); color: #fff;
  font: 600 13px/1 'Inter', sans-serif; cursor: pointer;
  transition: background .15s, border-color .15s;
}
.btn-primary:hover:not(:disabled) { background: var(--brand-ink); border-color: var(--brand-ink); }
.btn-primary:disabled { opacity: .5; cursor: not-allowed; }

.promo-modal-enter-active, .promo-modal-leave-active { transition: opacity .2s var(--ease); }
.promo-modal-enter-from, .promo-modal-leave-to { opacity: 0; }
.promo-modal-enter-active .promo-modal { transition: transform .25s var(--ease); }
.promo-modal-enter-from .promo-modal, .promo-modal-leave-to .promo-modal { transform: scale(.96) translateY(10px); }

/* ── Transition ── */
.panel-enter-active .panel-overlay,
.panel-leave-active .panel-overlay {
  transition: opacity .3s var(--ease);
}
.panel-enter-from .panel-overlay,
.panel-leave-to .panel-overlay { opacity: 0; }

.panel-enter-active .panel,
.panel-leave-active .panel {
  transition: transform .32s var(--ease);
}
.panel-enter-from .panel,
.panel-leave-to .panel { transform: translateX(100%); }
</style>
