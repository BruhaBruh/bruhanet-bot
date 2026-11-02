import { createRouter, createWebHistory } from 'vue-router'
import AdminLayout from './layout/AdminLayout.vue'
import LoginRequired from './pages/LoginRequired.vue'
import Payments from './pages/Payments.vue'
import Soon from './pages/Soon.vue'
import UserCard from './pages/UserCard.vue'
import Users from './pages/Users.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login-required', component: LoginRequired, meta: { title: 'Нужен вход' } },
    {
      path: '/',
      component: AdminLayout,
      redirect: '/users',
      children: [
        { path: 'users', component: Users, meta: { title: 'Юзеры' } },
        { path: 'users/:id', component: UserCard, meta: { title: 'Юзеры' } },
        { path: 'payments', component: Payments, meta: { title: 'Платежи' } },
        { path: 'broadcasts', component: Soon, meta: { title: 'Рассылки' } },
        { path: 'promos', component: Soon, meta: { title: 'Промокоды' } },
        { path: 'texts', component: Soon, meta: { title: 'Тексты' } },
        { path: 'settings', component: Soon, meta: { title: 'Настройки' } },
      ],
    },
    { path: '/:p(.*)*', redirect: '/users' },
  ],
})

router.afterEach((to) => {
  const t = to.meta.title
  document.title = typeof t === 'string' ? `${t} — BruhaNet` : 'BruhaNet'
})
