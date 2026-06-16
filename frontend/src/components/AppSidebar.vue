<script setup>
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

defineProps({ open: Boolean })
defineEmits(['close'])

const router = useRouter()
const auth = useAuthStore()

function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <div>
    <div class="sidebar-overlay" :class="{ active: open }" @click="$emit('close')" />

    <aside class="sidebar" :class="{ open }">
      <div class="sidebar-brand">
        <img src="../assets/icons/LogoLeftMenu.svg" alt="Логотип" class="sidebar-logo" />
        <span>Академический<br>Ассистент</span>
      </div>

      <nav class="sidebar-nav">

        <!-- Администратор -->
        <template v-if="auth.isAdmin">
          <div class="nav-section">
            <RouterLink class="nav-item" to="/admin">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 00-3-3.87M16 3.13a4 4 0 010 7.75"/>
              </svg>
              Управление ролями
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/attendance">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/><path d="M9 15l2 2 4-4"/>
              </svg>
              Авто-зачёт
            </RouterLink>
          </div>
        </template>

        <!-- Деканат -->
        <template v-else-if="auth.isDean">
          <div class="nav-section">
            <RouterLink class="nav-item" to="/dean">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/>
              </svg>
              Главная
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/retakes">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/>
              </svg>
              Пересдачи
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/requests">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M22 12h-6l-2 3H10l-2-3H2"/><path d="M5.45 5.11L2 12v6a2 2 0 002 2h16a2 2 0 002-2v-6l-3.45-6.89A2 2 0 0016.76 4H7.24a2 2 0 00-1.79 1.11z"/>
              </svg>
              Центр заявок
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/statements">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2"/><rect x="9" y="3" width="6" height="4" rx="2"/><path d="M9 12h6M9 16h4"/>
              </svg>
              Ведомость
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/users">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 00-3-3.87M16 3.13a4 4 0 010 7.75"/>
              </svg>
              Пользователи
            </RouterLink>
          </div>
        </template>

        <!-- Преподаватель -->
        <template v-else-if="auth.isTeacher">
          <div class="nav-section">
            <RouterLink class="nav-item" to="/teacher">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/>
              </svg>
              Главная
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/retakes">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/>
              </svg>
              Пересдачи
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/teacher-requests">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M22 12h-6l-2 3H10l-2-3H2"/><path d="M5.45 5.11L2 12v6a2 2 0 002 2h16a2 2 0 002-2v-6l-3.45-6.89A2 2 0 0016.76 4H7.24a2 2 0 00-1.79 1.11z"/>
              </svg>
              Центр заявок
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/statements">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2"/><rect x="9" y="3" width="6" height="4" rx="2"/><path d="M9 12h6M9 16h4"/>
              </svg>
              Ведомость
            </RouterLink>
          </div>
        </template>

        <!-- Студент -->
        <template v-else-if="auth.isStudent">
          <div class="nav-section">
            <RouterLink class="nav-item" to="/student">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/>
              </svg>
              Главная
            </RouterLink>
          </div>
          <div class="nav-section">
            <RouterLink class="nav-item" to="/retakes">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15"/>
              </svg>
              Пересдачи
            </RouterLink>
          </div>
        </template>

        <!-- Профиль — доступен всем ролям -->
        <div class="nav-section">
          <RouterLink class="nav-item" to="/profile">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>
            </svg>
            Профиль
          </RouterLink>
        </div>

      </nav>

      <div class="sidebar-footer">
        <button class="logout-btn" @click="logout">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9"/>
          </svg>
          Выйти
        </button>
      </div>
    </aside>
  </div>
</template>

<style scoped>
.sidebar-overlay {
  position: fixed; inset: 0;
  background: rgba(10,12,30,.45); z-index: 99;
  opacity: 0; pointer-events: none;
  transition: opacity .25s cubic-bezier(.2,.7,.2,1);
}
.sidebar-overlay.active { opacity: 1; pointer-events: all; }

.sidebar {
  position: fixed; top: 0; left: 0; bottom: 0; width: 280px;
  background: var(--card); box-shadow: 4px 0 24px rgba(20,22,60,.12);
  z-index: 100; display: flex; flex-direction: column;
  transform: translateX(-100%);
  transition: transform .3s cubic-bezier(.2,.7,.2,1);
}
.sidebar.open { transform: translateX(0); }

.sidebar-brand {
  padding: 20px 24px 24px;
  display: flex; flex-direction: column; align-items: center; gap: 12px;
  border-bottom: 1px solid var(--line);
  font-family: 'Gerhaus', 'Inter', sans-serif;
  font-size: 15px; font-weight: 700; text-transform: uppercase;
  letter-spacing: .06em; color: var(--brand-ink); line-height: 1.5; text-align: center;
}
.sidebar-logo { width: 25px; height: auto; }

.sidebar-nav {flex: 1;  padding: 12px 0; overflow: hidden;}
.nav-section { margin-bottom: 4px; }
.nav-item {
  display: flex; align-items: center; gap: 10px;
  width: 100%; text-align: left;
  background: none; border: none; padding: 10px 20px;
  font: 500 14px/1 'Inter', sans-serif; color: var(--ink-soft); cursor: pointer;
  text-decoration: none;
  transition: background .15s, color .15s;
}
.nav-item:hover { background: rgba(59,63,224,.05); color: var(--ink); }
.nav-item.router-link-active,
.nav-item.router-link-exact-active {
  background: rgba(60,56,182,.08);
  color: #3C38B6;
  font-weight: 600;
}
.nav-item svg { width: 18px; height: 18px; flex-shrink: 0; }

.sidebar-footer { padding: 16px 20px; border-top: 1px solid var(--line); }
.logout-btn {
  width: 100%; height: 40px; border: none;
  border-radius: var(--radius);
  background: linear-gradient(135deg, #2b5cff 0%, #5b3bd9 55%, #8b3df0 100%);
  color: #fff; font: 600 14px/1 'Inter', sans-serif; cursor: pointer;
  box-shadow: 0 8px 20px -8px rgba(91,59,217,.55);
  display: flex; align-items: center; justify-content: center; gap: 8px;
  transition: transform .2s, box-shadow .2s;
}
.logout-btn:hover { transform: scale(1.02); box-shadow: 0 4px 10px -5px rgba(91,59,217,.7); }
.logout-btn svg { width: 16px; height: 16px; flex-shrink: 0; }

@media (max-width: 600px) {
  .sidebar-brand { align-items: flex-start; text-align: left; }
}
</style>
