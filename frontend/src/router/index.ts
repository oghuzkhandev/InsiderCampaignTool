import { createRouter, createWebHistory } from 'vue-router'
import CampaignPreviewView from '@/views/CampaignPreviewView.vue'
import CampaignView from '@/views/CampaignView.vue'
import DeliveryDashboardView from '@/views/DeliveryDashboardView.vue'
import OverviewView from '@/views/OverviewView.vue'
import CampaignListView from '@/views/CampaignListView.vue'
import AudiencesView from '@/views/AudiencesView.vue'
import TemplatesView from '@/views/TemplatesView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    { path: '/', name: 'campaign-preview', component: CampaignPreviewView },
    { path: '/overview', name: 'overview', component: OverviewView },
    { path: '/campaigns', name: 'campaign-list', component: CampaignListView },
    { path: '/audiences', name: 'audiences', component: AudiencesView },
    { path: '/templates', name: 'templates', component: TemplatesView },
    { path: '/campaigns/:id', name: 'campaign', component: CampaignView, props: true },
    {
      path: '/campaigns/:id/delivery',
      name: 'campaign-delivery',
      component: DeliveryDashboardView,
      props: true,
    },
  ],
})

export default router
