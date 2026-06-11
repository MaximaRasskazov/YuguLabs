<script setup>
import { ref, computed } from 'vue'
import { useAuthStore } from '../stores/auth'
import ProfilePanel from './ProfilePanel.vue'

defineEmits(['open-sidebar'])

const auth = useAuthStore()
const profileOpen = ref(false)

const roleLabel = computed(() => ({
  DEAN: 'Деканат',
  TEACHER: 'Преподаватель',
  STUDENT: 'Студент',
  ADMIN: 'Администратор',
}[auth.role] ?? ''))
</script>

<template>
  <header class="header">
    <div class="header-left">
      <button class="burger" @click="$emit('open-sidebar')" aria-label="Открыть меню">
        <span /><span /><span />
      </button>
      <span class="app-name">Академический<br>Ассистент</span>
    </div>
    <div class="header-right">
      <div class="user-meta">
        <span class="user-name">{{ auth.user?.lastName }} {{ auth.user?.firstName }} {{ auth.user?.middleName }}</span>
        <span class="user-role">{{ roleLabel }}</span>
      </div>
      <button class="avatar" @click="profileOpen = true" aria-label="Открыть профиль">
        <img v-if="auth.user?.avatar" :src="auth.user.avatar" alt="Фото профиля" @error="auth.setAvatar(null)" />
        <span v-else>{{ auth.user?.firstName?.[0] }}{{ auth.user?.lastName?.[0] }}</span>
      </button>
    </div>
  </header>

  <ProfilePanel :open="profileOpen" @close="profileOpen = false" />
</template>

<style scoped>
.header {
  height: 64px;
  background: var(--card);
  border-bottom: 1px solid var(--line);
  display: flex; align-items: center; justify-content: space-between;
  padding: 0 24px;
  position: sticky; top: 0; z-index: 10; flex-shrink: 0;
}
.header-left { display: flex; align-items: center; gap: 16px; }
.burger {
  background: none; border: none; cursor: pointer; padding: 8px;
  display: flex; flex-direction: column; justify-content: center; gap: 3px;
  transition: background .15s;
}
.burger span {
  display: block; width: 22px; height: 2.5px;
  background:#3C38B6;
}
.app-name {
  font-family: 'Gerhaus', 'Inter', sans-serif;
  font-size: 13px; font-weight: 700; color: #3C38B6;
  text-transform: uppercase; letter-spacing: .04em; line-height: 1.25;
  text-align: left;
}
.header-right { display: flex; align-items: center; gap: 15px; }
.user-meta { display: flex; flex-direction: column; align-items: flex-end;}
.user-name { font-size: 14px; font-weight: 500; white-space: nowrap;}
.user-role { font-size: 12px; color: var(--ink-soft); }
.avatar {
  width: 40px; height: 40px; border-radius: 50%; flex-shrink: 0;
  background: linear-gradient(135deg, #2b5cff, #8b3df0);
  display: grid; place-items: center;
  color: #fff; font-weight: 700; font-size: 14px;
  border: none; cursor: pointer; overflow: hidden;
  transition: transform .2s, box-shadow .2s;
}
.avatar:hover { transform: scale(1.07); box-shadow: 0 4px 12px -4px rgba(91,59,217,.5); }
.avatar img { width: 100%; height: 100%; object-fit: cover; }

@media (max-width: 600px) {
  .user-meta { display: none; }
}
</style>
