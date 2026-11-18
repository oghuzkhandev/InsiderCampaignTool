import { createRouter, createWebHistory } from 'vue-router'
import CampaignPreviewView from '@/views/CampaignPreviewView.vue'
import CampaignView from '@/views/CampaignView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', name: 'campaign-preview', component: CampaignPreviewView },
    { path: '/campaigns/:id', name: 'campaign', component: CampaignView, props: true },
  ],
})

export default router
