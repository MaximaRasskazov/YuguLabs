// Утилиты скачивания бинарных ответов (blob) — для оригинала фото и
// ZIP-архива фотографий. Браузер не умеет «скачать» ответ axios напрямую:
// создаём временный object-URL и кликаем по скрытой ссылке.

/**
 * Сохраняет blob как файл (триггерит загрузку в браузере).
 * @param {Blob} blob
 * @param {string} filename
 */
export function saveBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename || 'download'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/**
 * Достаёт имя файла из заголовка Content-Disposition
 * (`attachment; filename="me.jpg"`), иначе возвращает fallback.
 * @param {string} header
 * @param {string} fallback
 * @returns {string}
 */
export function filenameFromDisposition(header, fallback) {
  const match = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(header || '')
  if (!match) return fallback
  try {
    return decodeURIComponent(match[1])
  } catch {
    return match[1]
  }
}
