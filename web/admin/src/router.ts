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
    { path: '/login-required', component: LoginRequired },
    {
      path: '/',
      component: AdminLayout,
      redirect: '/users',
      children: [
        { path: 'users', component: Users },
        { path: 'users/:id', component: UserCard },
        { path: 'payments', component: Payments },
        { path: 'broadcasts', component: Soon },
        { path: 'promos', component: Soon },
        { path: 'texts', component: Soon },
        { path: 'settings', component: Soon },
      ],
    },
  ],
})
