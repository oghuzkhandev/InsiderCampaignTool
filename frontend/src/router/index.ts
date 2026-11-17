import { createRouter, createWebHistory } from 'vue-router'
import CampaignPreviewView from '@/views/CampaignPreviewView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [{ path: '/', name: 'campaign-preview', component: CampaignPreviewView }],
})

export default router
