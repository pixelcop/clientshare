export const tailwindConfig = {
  theme: {
    extend: {
      colors: {
        brand: '#0ea5e9',
        'brand-dark': '#0284c7',
        'surface-bg': '#f3f4f6',
        'surface-card': '#ffffff',
        'surface-border': '#dbe3ef',
        'text-primary': '#334155',
        'text-muted': '#64748b',
      },
      boxShadow: {
        card: '0 1px 2px rgba(0, 0, 0, 0.06)',
      },
      fontFamily: {
        sans: [
          'Inter',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica',
          'Arial',
          'sans-serif',
        ],
      },
    },
    fontSize: {
      xs: ['12px', { lineHeight: '16px' }],
      sm: ['14px', { lineHeight: '20px' }],
      base: ['16px', { lineHeight: '24px' }],
      lg: ['18px', { lineHeight: '28px' }],
      xl: ['20px', { lineHeight: '28px' }],
      '2xl': ['24px', { lineHeight: '32px' }],
    },
  },
};
