import { PRODUCT, PAGE_TYPES } from './config/suseai';

export default [
  // Product root → redirect to Apps
  {
    name:     `c-cluster-${PRODUCT}-home-root`,
    path:     `/c/:cluster/${PRODUCT}`,
    redirect: { name: `c-cluster-${PRODUCT}-${PAGE_TYPES.APPS}`, params: { product: PRODUCT } },
    meta:     { product: PRODUCT }
  },

  // Apps page - Main application listing
  {
    name:      `c-cluster-${PRODUCT}-${PAGE_TYPES.APPS}`,
    path:      `/c/:cluster/${PRODUCT}/${PAGE_TYPES.APPS}`,
    component: () => import('./pages/Apps.vue'),
    meta:      { product: PRODUCT, category: 'apps' }
  },


  // Install flow (step-based wizard)
  {
    name:      `c-cluster-${PRODUCT}-${PAGE_TYPES.INSTALL}`,
    path:      `/c/:cluster/${PRODUCT}/${PAGE_TYPES.APPS}/:slug/${PAGE_TYPES.INSTALL}`,
    props:     true,
    component: () => import('./pages/Install.vue'),
    meta:      { product: PRODUCT, category: 'install' }
  },

  // App instances page - shows all instances of a specific app
  {
    name:      `c-cluster-${PRODUCT}-app-instances`,
    path:      `/c/:cluster/${PRODUCT}/${PAGE_TYPES.APPS}/:slug/instances`,
    props:     true,
    component: () => import('./pages/AppInstances.vue'),
    meta:      { product: PRODUCT, category: 'app-instances' }
  },

  // Manage flow (manage existing installation)
  {
    name:      `c-cluster-${PRODUCT}-${PAGE_TYPES.MANAGE}`,
    path:      `/c/:cluster/${PRODUCT}/${PAGE_TYPES.APPS}/:slug/${PAGE_TYPES.MANAGE}`,
    props:     true,
    component: () => import('./pages/Manage.vue'),
    meta:      { product: PRODUCT, category: 'manage' }
  },

  // Repositories management (future)
  {
    name:      `c-cluster-${PRODUCT}-${PAGE_TYPES.REPOSITORIES}`,
    path:      `/c/:cluster/${PRODUCT}/${PAGE_TYPES.REPOSITORIES}`,
    component: () => import('./pages/Apps.vue'), // Placeholder for now
    meta:      { product: PRODUCT, category: 'repositories' }
  },

  // Settings management (future)
  {
    name:      `c-cluster-${PRODUCT}-${PAGE_TYPES.SETTINGS}`,
    path:      `/c/:cluster/${PRODUCT}/${PAGE_TYPES.SETTINGS}`,
    component: () => import('./pages/Apps.vue'), // Placeholder for now
    meta:      { product: PRODUCT, category: 'settings' }
  },

  // Legacy routes (kept for compatibility during transition)
  {
    name:      `c-cluster-${PRODUCT}-home`,
    path:      `/c/:cluster/${PRODUCT}/home`,
    redirect:  { name: `c-cluster-${PRODUCT}-${PAGE_TYPES.APPS}`, params: { product: PRODUCT } },
    meta:      { product: PRODUCT }
  }
];
