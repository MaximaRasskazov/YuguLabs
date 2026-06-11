import { defineStore } from 'pinia'

function normalizeUser(u) {
  if (!u) return null
  return {
    id:         u.id,
    email:      u.email,
    firstName:  u.first_name  ?? u.firstName  ?? '',
    lastName:   u.last_name   ?? u.lastName   ?? '',
    middleName: u.middle_name ?? u.middleName ?? '',
    group:      u.group_name  ?? u.group      ?? '',
    avatar:     u.avatar_url  ?? u.avatar     ?? null,
  }
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || null,
    role: localStorage.getItem('role') || null,
    user: JSON.parse(localStorage.getItem('user') || 'null'),
  }),

  getters: {
    isLoggedIn: (state) => !!state.token,
    isStudent: (state) => state.role === 'STUDENT',
    isTeacher: (state) => state.role === 'TEACHER',
    isDean: (state) => state.role === 'DEAN',
    isAdmin: (state) => state.role === 'ADMIN',
  },

  actions: {
    login({ token, role, user }) {
      this.token = token
      this.role = role
      this.user = normalizeUser(user)

      localStorage.setItem('token', token)
      localStorage.setItem('role', role)
      localStorage.setItem('user', JSON.stringify(this.user))
    },

    logout() {
      this.token = null
      this.role = null
      this.user = null

      localStorage.removeItem('token')
      localStorage.removeItem('role')
      localStorage.removeItem('user')
    },

    // Обновляет аватар текущего пользователя (после загрузки/удаления)
    // и синхронизирует localStorage, чтобы фото не пропадало при перезагрузке.
    setAvatar(url) {
      if (!this.user) return
      this.user = { ...this.user, avatar: url }
      localStorage.setItem('user', JSON.stringify(this.user))
    },

    // Вызывается при старте приложения — восстанавливает сессию из хранилища
    restore() {
      this.token = localStorage.getItem('token') || null
      this.role  = localStorage.getItem('role')  || null
      this.user  = JSON.parse(localStorage.getItem('user') || 'null')
    },
  },
})
