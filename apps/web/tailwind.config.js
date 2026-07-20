/** @type {import('tailwindcss').Config} */
// O theme referencia as CSS custom properties de src/styles/tokens.css (fonte
// de verdade). Regra do DS: NÃO usar modificador /alpha sobre cores de token
// (ex.: bg-primary-300/50) — var() simples não suporta alpha; use os tints
// 100/200. Estados pressed/disabled usam --opacity-pressed / --opacity-disabled.
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        primary: {
          100: 'var(--color-primary-100)',
          200: 'var(--color-primary-200)',
          300: 'var(--color-primary-300)',
        },
        accent: {
          100: 'var(--color-accent-100)',
          200: 'var(--color-accent-200)',
          300: 'var(--color-accent-300)',
        },
        ink: 'var(--color-text)',
        muted: 'var(--color-gray-300)',
        page: 'var(--color-gray-50)',
        success: 'var(--color-success)',
        error: 'var(--color-error)',
        alert: 'var(--color-alert)',
      },
      fontFamily: {
        sans: ['Nunito', '-apple-system', 'Segoe UI', 'Roboto', 'sans-serif'],
      },
      // Sobrescreve rounded-sm/md/lg do Tailwind — intencional: o DS usa só
      // estes raios (8 inputs, 12 blocos internos, 16 cards/botões, 999 pills).
      borderRadius: {
        sm: '8px',
        md: '12px',
        lg: '16px',
        pill: '999px',
      },
      boxShadow: {
        card: 'var(--shadow-card)',
        raised: 'var(--shadow-raised)',
        button: 'var(--shadow-button)',
      },
      maxWidth: {
        shell: '1240px',
      },
      // Spacing: não customizado — a escala default de 4px do Tailwind já cobre.
    },
  },
  plugins: [],
};
