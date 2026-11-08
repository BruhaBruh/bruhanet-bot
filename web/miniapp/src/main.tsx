import './styles.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'

// В dev вне Telegram (нет подписанного initData) подставляем моки до первого запроса; внутри Telegram идёт настоящий API.
// В прод-сборке ветка вырезается целиком.
const prepare =
  import.meta.env.DEV && !window.Telegram?.WebApp?.initData ? import('./dev/mock').then((m) => m.installMock()) : Promise.resolve()

prepare.then(() => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
})
