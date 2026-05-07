import { storeToRefs } from 'pinia';

import { useAuthStore } from '@/stores/auth';

const layoutConfig = reactive({
  preset: 'Aura',
  primary: 'emerald',
  surface: null,
  darkTheme: false,
});

const layoutState = reactive({
  menuHoverActive: false,
  activeMenuItem: null,
  activePath: null,
});

export function useLayout() {
  const { user } = storeToRefs(useAuthStore());
  const isPublicLayout = computed(() =>
    ['link', 'client', 'customer'].includes(user.value?.role ?? ''),
  );

  const toggleDarkMode = () => {
    if (!document.startViewTransition) {
      executeDarkModeToggle();
      return;
    }

    document.startViewTransition(() => executeDarkModeToggle());
  };

  const executeDarkModeToggle = () => {
    layoutConfig.darkTheme = !layoutConfig.darkTheme;
    document.documentElement.classList.toggle('app-dark');
    localStorage.setItem('color-mode', layoutConfig.darkTheme ? 'dark' : 'light');
  };

  const isDarkTheme = computed(() => layoutConfig.darkTheme);
  const isDesktop = () => window.innerWidth > 991;

  onMounted(() => {
    if (localStorage.getItem('color-mode')) {
      layoutConfig.darkTheme = localStorage.getItem('color-mode') === 'dark';
    }
    if (layoutConfig.darkTheme) {
      document.documentElement.classList.add('app-dark');
    }
  });

  return {
    layoutConfig,
    layoutState,
    isDarkTheme,
    toggleDarkMode,
    isDesktop,

    isPublicLayout,
  };
}
