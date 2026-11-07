// 404 (нет фото, нет в users, Telegram молчит) помним до перезагрузки страницы: не повторяем на каждый рендер.
export const failedAvatars = new Set<string>()
