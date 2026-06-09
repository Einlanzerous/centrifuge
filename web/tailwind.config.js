import forms from '@tailwindcss/forms'
import typography from '@tailwindcss/typography'

/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{vue,js,ts}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Geist"', 'system-ui', 'sans-serif'],
        serif: ['"Instrument Serif"', 'Georgia', 'serif'],
        mono: ['"Geist Mono"', 'ui-monospace', 'monospace'],
      },
      colors: {
        // Warm, low-saturation neutral canvas (design system).
        ink: {
          50: '#f7f5f1',
          100: '#efece5',
          200: '#dfdbd0',
          300: '#bcb6a8',
          400: '#8c8576',
          500: '#605a4e',
          600: '#3f3b32',
          700: '#2a2722',
          800: '#1a1814',
          900: '#0f0d0a',
        },
        // Fixed UI-accent ramps used for action states (bookmark / thumbs).
        // NOT the topic taxonomy — dynamic per-topic colors come from the API
        // (topic_color, hashed hue). See lib/art.ts and the CTFG-28 guardrail.
        ai: { 50: '#fff5e6', 200: '#ffd699', 500: '#e88a16', 700: '#a5570b', ink: '#3a1d04' },
        transit: { 50: '#e6fbf7', 200: '#9fe9d8', 500: '#0fb39b', 700: '#067a6a', ink: '#03332c' },
        game: { 50: '#fde9f3', 200: '#f3a8cf', 500: '#d63c8c', 700: '#9c1d62', ink: '#3a0a23' },
      },
    },
  },
  plugins: [forms, typography],
}
