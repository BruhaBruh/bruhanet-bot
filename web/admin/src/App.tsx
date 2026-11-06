import { lazy, useEffect } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router'
import { ToastProvider } from '@/ui'
import { setUnauthorizedHandler } from './api'
import { AppShell } from './layout/AppShell'
import { sectionOf } from './layout/nav'
import LoginRequired from './pages/LoginRequired'
import Overview from './pages/Overview'
import Payments from './pages/Payments'
import PromoNew from './pages/PromoNew'
import Promos from './pages/Promos'
import Settings from './pages/Settings'
import UserCard from './pages/UserCard'
import Users from './pages/Users'
import { useTheme } from './theme'

// tiptap — отдельным чанком
const Broadcasts = lazy(() => import('./pages/Broadcasts'))
const BroadcastNew = lazy(() => import('./pages/BroadcastNew'))
const Texts = lazy(() => import('./pages/Texts'))

const title = (pathname: string) =>
  `${pathname === '/login-required' ? 'Вход в админ-панель' : sectionOf(pathname).label} — BruhaNet`

export function App() {
  useTheme()
  const navigate = useNavigate()
  const { pathname } = useLocation()

  useEffect(() => setUnauthorizedHandler(() => navigate('/login-required', { replace: true })), [navigate])
  useEffect(() => {
    document.title = title(pathname)
  }, [pathname])

  return (
    <ToastProvider>
      <Routes>
        <Route path="/login-required" element={<LoginRequired />} />
        <Route element={<AppShell />}>
          <Route path="/" element={<Overview />} />
          <Route path="/broadcasts" element={<Broadcasts />} />
          <Route path="/broadcasts/new" element={<BroadcastNew />} />
          <Route path="/promos" element={<Promos />} />
          <Route path="/promos/new" element={<PromoNew />} />
          <Route path="/users" element={<Users />} />
          <Route path="/users/:id" element={<UserCard />} />
          <Route path="/payments" element={<Payments />} />
          <Route path="/texts" element={<Texts />} />
          <Route path="/settings" element={<Settings />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ToastProvider>
  )
}
