import { describe, it, expect } from 'vitest'
import { filenameFromDisposition } from './download'

describe('filenameFromDisposition', () => {
  it('достаёт имя из кавычек', () => {
    expect(filenameFromDisposition('attachment; filename="me.jpg"', 'x')).toBe('me.jpg')
  })

  it('достаёт имя без кавычек', () => {
    expect(filenameFromDisposition('attachment; filename=report.zip', 'x')).toBe('report.zip')
  })

  it('декодирует percent-encoding', () => {
    expect(filenameFromDisposition("attachment; filename*=UTF-8''%D1%84%D0%BE%D1%82%D0%BE.jpg", 'x'))
      .toBe('фото.jpg')
  })

  it('возвращает fallback при пустом/некорректном заголовке', () => {
    expect(filenameFromDisposition('', 'fallback.bin')).toBe('fallback.bin')
    expect(filenameFromDisposition(undefined, 'fallback.bin')).toBe('fallback.bin')
  })
})
