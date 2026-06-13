import http from './http'

export const authApi = {
  login: (email, password) =>
    http.post('/api/auth/login', { email, password }),

  register: (data) =>
    http.post('/api/auth/register', data),

  logout: () =>
    http.post('/api/auth/logout'),

  me: () =>
    http.get('/api/auth/me'),

  refresh: () =>
    http.post('/api/auth/refresh'),

  changePassword: (currentPassword, newPassword) =>
    http.post('/api/me/password', { current_password: currentPassword, new_password: newPassword }),

  // Точечное обновление профиля (PATCH-семантика): ФИО, группа, дата
  // рождения. Передавайте только меняемые поля; бэкенд логирует diff в
  // change_logs. Возвращает обновлённый ProfileResponse ({ user, ... }).
  updateProfile: (data) =>
    http.patch('/api/me', data),

  // Фотография профиля. uploadAvatar шлёт multipart (поле «photo»),
  // возвращает { avatar_url, ... }. Бэкенд проверяет подлинность, сжимает
  // оригинал и делает аватар 128×128. deleteAvatar убирает текущую.
  uploadAvatar: (file) => {
    const fd = new FormData()
    fd.append('photo', file)
    return http.post('/api/me/avatar', fd)
  },

  deleteAvatar: () =>
    http.delete('/api/me/avatar'),

  // Скачать оригинал своей фотографии (защищённый маршрут, бинарь).
  downloadOriginal: () =>
    http.get('/api/photo/download', { responseType: 'blob' }),
}
