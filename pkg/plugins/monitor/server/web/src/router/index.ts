import { RouteParam } from '@/types/router'
import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: () => import('@/views/home/Page.vue')
    },
    {
      path: '/nodes/:' + RouteParam.visualization,
      name: 'nodes',
      component: () => import('@/views/nodes/Page.vue')
    },
  ]
})

export default router
