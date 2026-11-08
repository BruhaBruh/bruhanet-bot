import './styles.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'

// В dev подставляем моки до первого запроса; в прод-сборке ветка вырезается целиком.
const prepare = import.meta.env.DEV ? import('./dev/mock').then((m) => m.installMock()) : Promise.resolve()

prepare.then(() => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
})
