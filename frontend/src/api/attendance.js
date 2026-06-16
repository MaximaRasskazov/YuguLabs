import http from './http'

// API лабы №12 — авто-зачёт по файлу успеваемости.
// calculate шлёт .xlsx как multipart (поле «file», как ждёт бэкенд) и
// возвращает отчёт: группы со студентами + плоский список зачётников.
export const attendanceApi = {
  calculate: (file) => {
    const fd = new FormData()
    fd.append('file', file)
    return http.post('/api/attendance/calculate', fd, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
}
