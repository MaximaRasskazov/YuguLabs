import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

import LoginPage from '../views/LoginPage.vue'
import RegisterPage from '../views/RegisterPage.vue'
import StudentPage from '../views/StudentPage.vue'
import RetakesPage from '../views/RetakesPage.vue'
import RetakeCreatePage from '../views/RetakeCreatePage.vue'
import RequestsPage from '../views/RequestsPage.vue'
import DeanPage from '../views/DeanPage.vue'
import TeacherPage from '../views/TeacherPage.vue'
import TeacherRequestsPage from '../views/TeacherRequestsPage.vue'
import StatementsPage from '../views/StatementsPage.vue'
import UsersPage from '../views/UsersPage.vue'
import AdminPage from '../views/AdminPage.vue'
import AttendancePage from '../views/AttendancePage.vue'
import ProfilePage from '../views/ProfilePage.vue'

const HOME = { STUDENT: '/student', TEACHER: '/teacher', DEAN: '/dean', ADMIN: '/admin' }
const homeFor = (role) => HOME[role] ?? '/login'

const routes = [
  { path: '/', redirect: () => homeFor(useAuthStore().role) },
  { path: '/login', component: LoginPage, meta: { guest: true } },
  { path: '/register', component: RegisterPage, meta: { guest: true } },
  { path: '/student', component: StudentPage, meta: { auth: true, roles: ['STUDENT'] } },
  { path: '/retakes', component: RetakesPage, meta: { auth: true } },
  { path: '/retakes/create', component: RetakeCreatePage, meta: { auth: true, roles: ['DEAN'] } },
  { path: '/requests', component: RequestsPage, meta: { auth: true, roles: ['DEAN'] } },
  { path: '/dean', component: DeanPage, meta: { auth: true, roles: ['DEAN'] } },
  { path: '/teacher', component: TeacherPage, meta: { auth: true, roles: ['TEACHER'] } },
  { path: '/teacher-requests', component: TeacherRequestsPage, meta: { auth: true, roles: ['TEACHER'] } },
  { path: '/statements', component: StatementsPage, meta: { auth: true, roles: ['TEACHER', 'DEAN'] } },
  { path: '/users', component: UsersPage, meta: { auth: true } },
  { path: '/admin', component: AdminPage, meta: { auth: true, roles: ['ADMIN'] } },
  { path: '/attendance', component: AttendancePage, meta: { auth: true, roles: ['ADMIN'] } },
  { path: '/profile', component: ProfilePage, meta: { auth: true } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()

  if (to.meta.auth && !auth.isLoggedIn) {
    return '/login'
  }

  if (to.meta.guest && auth.isLoggedIn) {
    return homeFor(auth.role)
  }

  if (to.meta.roles && !to.meta.roles.includes(auth.role)) {
    return homeFor(auth.role)
  }
})

export default router
