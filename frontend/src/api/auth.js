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

  // Аватар профиля. uploadAvatar шлёт multipart (поле «avatar»),
  // возвращает { avatar_url }. deleteAvatar убирает текущий.
  uploadAvatar: (file) => {
    const fd = new FormData()
    fd.append('avatar', file)
    return http.post('/api/me/avatar', fd)
  },

  deleteAvatar: () =>
    http.delete('/api/me/avatar'),
}
