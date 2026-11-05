import { AppRoot } from '@telegram-apps/telegram-ui'
import { lazy, useEffect } from 'react'
import { Navigate, Outlet, Route, Routes, useLocation, useNavigate } from 'react-router'
import { ToastProvider } from '@/ui'
import { setUnauthorizedHandler } from './api'
import { AppShell } from './layout/AppShell'
import { sectionOf } from './layout/nav'
import BroadcastNew from './pages/BroadcastNew'
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
const Texts = lazy(() => import('./pages/Texts'))

const title = (pathname: string) =>
  `${pathname === '/login-required' ? 'Нужен вход' : sectionOf(pathname).label} — BruhaNet`

export function App() {
  const { theme } = useTheme()
  const navigate = useNavigate()
  const { pathname } = useLocation()

  useEffect(() => setUnauthorizedHandler(() => navigate('/login-required')), [navigate])
  useEffect(() => {
    document.title = title(pathname)
  }, [pathname])

  return (
    <ToastProvider>
      <Routes>
        <Route
          path="/login-required"
          element={
            <AppRoot className="app-root" appearance={theme} platform="ios">
              <LoginRequired />
            </AppRoot>
          }
        />
        <Route element={<AppShell />}>
          <Route path="/" element={<Overview />} />
          <Route path="/broadcasts/new" element={<BroadcastNew />} />
          <Route path="/promos/new" element={<PromoNew />} />
          {/* Старые экраны на telegram-ui: при замене экран переезжает из этого блока выше. */}
          <Route
            element={
              <AppRoot className="app-root in-shell" appearance={theme} platform="ios">
                <div className="col">
                  <Outlet />
                </div>
              </AppRoot>
            }
          >
            <Route path="/users" element={<Users />} />
            <Route path="/users/:id" element={<UserCard />} />
            <Route path="/payments" element={<Payments />} />
            <Route path="/broadcasts" element={<Broadcasts />} />
            <Route path="/promos" element={<Promos />} />
            <Route path="/texts" element={<Texts />} />
            <Route path="/settings" element={<Settings />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ToastProvider>
  )
}
