// Только для `pnpm dev`: подменяет Telegram и /m/api/*, чтобы смотреть вёрстку без бота и панели.
// Импортируется под import.meta.env.DEV и в прод-бандл не попадает (проверка — grep по dist: маркер ниже).
export const MOCK_MARKER = '__BRUHANET_MOCK__'

const day = 24 * 3600 * 1000
const iso = (offset: number) => new Date(Date.now() + offset).toISOString()

const routes: Record<string, unknown> = {
  '/m/api/me': { name: 'Бруха', username: 'bruha', botUsername: 'bruhanet_bot', blocked: false, isAdmin: true },
  '/m/api/subscription': {
    state: 'active', expireAt: iso(12 * day), daysLeft: 12, trafficUsedBytes: 12 * 1024 ** 3, trafficLimitBytes: 100 * 1024 ** 3,
    devices: { used: 2, limit: 5 },
  },
  '/m/api/referral': { enabled: true, link: 'https://t.me/bruhanet_bot?start=ref_42', days: 7, invited: 5, paid: 2, rewardDays: 14 },
  '/m/api/promos': { items: [{ code: 'WELCOME', kind: 'days', value: 7, activatedAt: iso(-3 * day) }] },
  '/m/api/devices': {
    limit: 5,
    items: [
      { hwid: 'hw-1', platform: 'iOS', osVersion: '18.1', deviceModel: 'iPhone 15', userAgent: 'Happ/2', createdAt: iso(-30 * day), updatedAt: iso(-day) },
      { hwid: 'hw-2', platform: 'Windows', osVersion: '11', deviceModel: null, userAgent: null, createdAt: iso(-10 * day), updatedAt: iso(-2 * day) },
    ],
  },
  '/m/api/instructions': {
    platforms: [
      {
        id: 'ios', title: 'iOS',
        apps: [{
          name: 'Happ', featured: true,
          blocks: [
            { title: 'Установка приложения', text: 'Откройте страницу в App Store и установите приложение.', icon: 'download', tone: 'cyan', buttons: [{ text: 'App Store', url: 'https://apps.apple.com/', kind: 'link' }] },
            { title: 'Добавление подписки', text: 'Нажмите кнопку ниже, чтобы добавить подписку.', icon: 'cloud', tone: 'teal', buttons: [{ text: 'Добавить подписку', url: 'happ://add/{{SUBSCRIPTION_LINK}}', kind: 'subscription' }] },
            { title: 'Подключение и использование', text: 'Выберите сервер и включите VPN большой кнопкой.', icon: 'check', tone: 'teal', buttons: [] },
          ],
        }],
      },
      { id: 'android', title: 'Android', apps: [{ name: 'V2Box', featured: false, blocks: [{ title: 'Установка', text: 'Скачайте из Google Play.', icon: 'download', tone: 'cyan', buttons: [] }] }] },
    ],
  },
  '/m/api/hosts': {
    items: [
      { id: 'aaaaaaaaaaaaaaa1', name: '⚡️ Авто', protocol: 'vless' },
      { id: 'aaaaaaaaaaaaaaa2', name: '📺 Авто YouTube', protocol: 'vless' },
      { id: 'aaaaaaaaaaaaaaa3', name: '🇩🇪 Hysteria 2', protocol: 'hysteria2' },
    ],
  },
}

export function installMock() {
  window.Telegram = {
    WebApp: {
      initData: 'mock', colorScheme: 'light', ready() {}, expand() {}, close() {}, openLink: (u: string) => window.open(u), openTelegramLink: (u: string) => window.open(u),
    },
  }
  const real = window.fetch.bind(window)
  window.fetch = async (input, init) => {
    const path = typeof input === 'string' ? input : input instanceof URL ? input.pathname : input.url
    if (!path.startsWith('/m/api/')) return real(input, init)
    if (path === '/m/api/admin/login-link') return Response.json({ url: 'https://admin.example.test/auth?token=__BRUHANET_MOCK__' })
    if (path === '/m/api/hosts/link') return Response.json({ url: 'vless://mock@host.example:443#mock' })
    if (path === '/m/api/subscription-link') return Response.json({ url: 'https://sub.example.test/mock-subscription' })
    const body = routes[path]
    return body === undefined ? new Response(null, { status: 204 }) : Response.json(body)
  }
}
