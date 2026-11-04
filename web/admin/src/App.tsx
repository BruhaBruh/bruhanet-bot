import { AppRoot, Spinner } from '@telegram-apps/telegram-ui'
import { lazy, Suspense, useEffect } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router'
import { setUnauthorizedHandler } from './api'
import { AdminLayout, sections } from './layout/AdminLayout'
import LoginRequired from './pages/LoginRequired'
import Payments from './pages/Payments'
import Promos from './pages/Promos'
import Settings from './pages/Settings'
import UserCard from './pages/UserCard'
import Users from './pages/Users'
import { useAppearance } from './theme'

// tiptap — отдельным чанком
const Broadcasts = lazy(() => import('./pages/Broadcasts'))
const Texts = lazy(() => import('./pages/Texts'))

function title(pathname: string) {
  if (pathname === '/login-required') return 'Нужен вход — BruhaNet'
  const s = sections.find((s) => pathname === s.to || pathname.startsWith(`${s.to}/`))
  return s ? `${s.label} — BruhaNet` : 'BruhaNet'
}

export function App() {
  const appearance = useAppearance()
  const navigate = useNavigate()
  const { pathname } = useLocation()

  useEffect(() => setUnauthorizedHandler(() => navigate('/login-required')), [navigate])
  useEffect(() => {
    document.title = title(pathname)
  }, [pathname])

  return (
    <AppRoot className="app-root" appearance={appearance} platform="ios">
      <Suspense fallback={<Spinner size="m" />}>
        <Routes>
          <Route path="/login-required" element={<LoginRequired />} />
          <Route element={<AdminLayout />}>
            <Route path="/users" element={<Users />} />
            <Route path="/users/:id" element={<UserCard />} />
            <Route path="/payments" element={<Payments />} />
            <Route path="/broadcasts" element={<Broadcasts />} />
            <Route path="/promos" element={<Promos />} />
            <Route path="/texts" element={<Texts />} />
            <Route path="/settings" element={<Settings />} />
          </Route>
          <Route path="*" element={<Navigate to="/users" replace />} />
        </Routes>
      </Suspense>
    </AppRoot>
  )
}
