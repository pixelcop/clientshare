import { createRouter, createWebHistory } from 'vue-router';

import ClientFiles from '@/views/ClientFiles.vue';

import { useAuthStore } from '../stores/auth';
import AcceptAccessView from '../views/AcceptAccessView.vue';
import AcceptInviteView from '../views/AcceptInviteView.vue';
import ActivityFeed from '../views/ActivityFeed.vue';
import ClientsList from '../views/ClientsList.vue';
import FileView from '../views/FileView.vue';
import ForgotPasswordView from '../views/ForgotPasswordView.vue';
import LoginView from '../views/LoginView.vue';
import ResetPasswordView from '../views/ResetPasswordView.vue';
import SecureLinks from '../views/SecureLinks.vue';
import SecurityView from '../views/SecurityView.vue';
import SettingsView from '../views/SettingsView.vue';
import SharedWithMe from '../views/SharedWithMe.vue';
import UsersList from '../views/UsersList.vue';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: LoginView,
      meta: { public: true },
      beforeEnter: (to, _from) => {
        const { user } = useAuthStore();
        if (user) {
          // If already logged in, redirect to home or intended page
          let redirect = to.query.redirect as string;
          if (!redirect && ['client', 'customer', 'link'].includes(user.role)) {
            redirect = '/files';
          }
          return redirect || { name: 'Clients' };
        }
        return true;
      },
    },
    {
      path: '/forgot-password',
      name: 'ForgotPassword',
      component: ForgotPasswordView,
      meta: { public: true },
    },
    {
      path: '/reset-password',
      name: 'ResetPassword',
      component: ResetPasswordView,
      meta: { public: true },
    },
    {
      path: '/accept-invite',
      name: 'AcceptInvite',
      component: AcceptInviteView,
      meta: { public: true },
    },
    {
      path: '/accept-access',
      name: 'AcceptAccess',
      component: AcceptAccessView,
      meta: { public: true },
    },
    {
      path: '/shared-with-me',
      name: 'SharedWithMe',
      component: SharedWithMe,
      meta: { public: false, roles: ['admin', 'manager', 'client', 'customer'] },
    },
    {
      path: '/clients',
      name: 'Clients',
      component: ClientsList,
      meta: { public: false },
    },
    {
      path: '/activity',
      name: 'Activity',
      component: ActivityFeed,
      meta: { public: false },
    },
    {
      path: '/folder/:folderId',
      alias: '/folders/:folderId',
      name: 'FolderView',
      component: ClientFiles,
      meta: { public: false, roles: ['admin', 'manager', 'client', 'customer', 'link'] },
      props: true,
    },
    {
      path: '/folder',
      name: 'FolderRootView',
      component: ClientFiles,
      meta: { public: false, roles: ['admin', 'manager', 'client', 'customer', 'link'] },
      props: { folderId: null },
    },
    {
      path: '/files',
      name: 'ClientFiles',
      component: ClientFiles,
      meta: { public: false, roles: ['admin', 'manager', 'client', 'customer', 'link'] },
      beforeEnter: (_to, _from) => {
        const { user } = useAuthStore();
        if (user?.role === 'link' || user?.role === 'client' || user?.role === 'customer') {
          return { name: 'FolderRootView' };
        }
        // If not a client, redirect to clients list
        return { name: 'Clients' };
      },
    },
    {
      path: '/files/:fileId',
      name: 'FileView',
      component: FileView,
      meta: { public: false },
      props: true,
    },
    {
      path: '/document-requests',
      name: 'MyDocumentRequests',
      component: () => import('../views/MyDocumentRequests.vue'),
      meta: { public: false, roles: ['client', 'customer'] },
    },
    {
      path: '/document-request',
      name: 'DocumentRequestRecipient',
      component: () => import('../views/DocumentRequestRecipient.vue'),
      meta: { public: true },
    },
    {
      path: '/clients/:clientId/document-requests/:requestId?',
      name: 'DocumentRequests',
      component: () => import('../views/DocumentRequests.vue'),
      props: true,
      meta: { public: false, roles: ['admin', 'manager'] },
    },
    {
      path: '/clients/:id/links',
      name: 'SecureLinks',
      component: SecureLinks,
      meta: { public: false, roles: ['admin', 'manager'] },
    },
    {
      path: '/users',
      name: 'Users',
      component: UsersList,
      meta: { public: false, roles: ['admin', 'manager'] },
      props: (route) => ({
        clientId: typeof route.query.client_id === 'string' ? route.query.client_id : '',
      }),
    },
    {
      path: '/settings',
      name: 'Settings',
      component: SettingsView,
      meta: { public: false, roles: ['admin'] },
    },
    {
      path: '/settings/access',
      name: 'AccessSettings',
      redirect: (to) => ({ name: 'Settings', query: { ...to.query, tab: 'access' } }),
      meta: { public: false, roles: ['admin'] },
    },
    {
      path: '/security',
      name: 'Security',
      component: SecurityView,
      meta: { public: false, roles: ['admin', 'manager', 'client', 'customer'] },
    },
  ],
});

let initialLoad = false;
router.beforeEach(async (_to, _from) => {
  // Always try restoring the first session load from the auth cookie.
  if (!initialLoad) {
    try {
      const auth = useAuthStore();
      if (!auth.user) {
        await auth.fetchMe();
      }
    } finally {
      initialLoad = true;
    }
  }
});

router.beforeEach(async (to, _from) => {
  const auth = useAuthStore();
  // If route is public, always allow
  if (to.meta.public) {
    return true;
  }
  // If user is logged in, allow
  if (auth.user) {
    if (to.name === 'Users' && !auth.canManageUsers) {
      return { name: 'Clients' };
    }
    const allowedRoles = to.meta.roles as string[] | undefined;
    if (allowedRoles && !allowedRoles.includes(auth.user.role)) {
      return ['client', 'customer', 'link'].includes(auth.user.role)
        ? { name: 'FolderRootView' }
        : { name: 'Clients' };
    }
    if (to.path === '/') {
      if (['client', 'customer', 'link'].includes(auth.user.role)) {
        return { name: 'ClientFiles' };
      }
      if (auth.user.role === 'admin' || auth.user.role === 'manager') {
        return { name: 'Clients' };
      }
    }
    return true;
  }
  // Otherwise, redirect to login
  return { name: 'Login', query: { redirect: to.fullPath } };
});

export default router;
