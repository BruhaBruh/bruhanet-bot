export interface UserSummary {
  telegramId: number
  username: string
  createdAt: string
  confirmed: number
  confirmedSum: number
}

export interface Payment {
  id: number | string
  telegramId: number
  username: string
  month: number
  price: number
  status: string
  prevExpireAt: string | null
  advanceGranted: boolean
  adminMessageId: number | null
  createdAt: string
  submittedAt: string | null
  resolvedAt: string | null
}

export interface UserDetail {
  user: UserSummary
  stats: {
    confirmed: number
    confirmedSum: number
    unpaidAdvances: number
    lastConfirmedAt: string | null
    firstConfirmedAt: string | null
    hasHistory: boolean
  }
  payments: Payment[]
  panel: { expireAt: string | null; status: string | null; error: 'panel_unavailable' | null }
}
