import { Placeholder } from '@telegram-apps/telegram-ui'

export default function LoginRequired() {
  return (
    <div className="login-box">
      <Placeholder header="Нужен вход" description="Отправь боту /login">
        <span className="login-emoji" aria-hidden="true">
          🔒
        </span>
      </Placeholder>
    </div>
  )
}
