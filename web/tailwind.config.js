/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      fontFamily: {
        display: ['"Avenir Next"', '"Segoe UI"', '"PingFang SC"', 'system-ui', 'sans-serif'],
        body: ['-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', '"PingFang SC"', '"Microsoft YaHei"', 'sans-serif'],
        mono: ['ui-monospace', '"SFMono-Regular"', 'Consolas', '"Liberation Mono"', 'monospace'],
      },
      colors: {
        // ── Brand: Luxurious Editorial Gold ──
        brand: {
          DEFAULT: '#c9954a',
          50:  '#fdf9f4',
          100: '#fbeedf',
          200: '#f6dbb9',
          300: '#efc187',
          400: '#e5a153',
          500: '#c9954a',
          600: '#b07f3c',
          700: '#926630',
          800: '#755027',
          900: '#5e4121',
          950: '#332110',
        },
        // ── Sage: Muted Teal Accent ──
        sage: {
          DEFAULT: '#6b8275',
          50:  '#f4f6f5',
          100: '#e5eae7',
          200: '#ccd7d1',
          300: '#a3b8ad',
          400: '#6b8275',
          500: '#54685d',
          600: '#425149',
          700: '#35413a',
          800: '#2d3531',
          950: '#141a17',
        },
        // ── Ink: High-Contrast Graphite / Charcoal (Elegant Light-Theme Text) ──
        ink: {
          DEFAULT: '#111827',
          50:  '#6b7280', // Elegant secondary text
          100: '#4b5563', // Main body text
          200: '#374151', // Dark text
          300: '#1f2937',
          400: '#111827',
          500: '#111827',
          600: '#111827', // Headers
          700: '#030712',
          800: '#000000',
          900: '#000000',
        },
        // ── Sand: High-End Swiss Slate Grays ──
        sand: {
          DEFAULT: '#f9fafb',
          50:  '#ffffff', // Card base
          100: '#f9fafb', // System body background
          200: '#f3f4f6', // Panel base / hover background
          300: '#e5e7eb', // Thin premium borders
          400: '#d1d5db', // Mid-lines
          500: '#9ca3af', // Muted placeholders
          600: '#6b7280',
          700: '#4b5563',
          800: '#374151',
          900: '#1f2937',
        },
        // ── Surface: Compatibility ──
        surface: {
          DEFAULT: '#ffffff',
          50:  '#ffffff',
          100: '#f9fafb',
          200: '#f3f4f6',
          300: '#e5e7eb',
          400: '#ffffff',
          500: '#ffffff',
          600: '#1f2937',
          700: '#111827',
          800: '#030712',
          900: '#000000',
          950: '#000000',
        },
        // Backward compatibility colors mapped to slate-light
        primary: {
          400: '#e5a153',
          500: '#c9954a',
          600: '#b07f3c',
        },
        accent: {
          400: '#6b8275',
          500: '#54685d',
        },
        cream: {
          DEFAULT: '#faf9f6',
          50:  '#ffffff',
          100: '#faf9f6',
          200: '#f4f3ee',
          300: '#e8e6dc',
          400: '#9ca3af',
          500: '#6b7280',
          600: '#4b5563',
          700: '#1f2937',
          800: '#111827',
          900: '#000000',
        },
      },
      fontSize: {
        '2xs': ['0.625rem', { lineHeight: '0.875rem' }],
      },
      spacing: {
        '18': '4.5rem',
        '88': '22rem',
      },
      transitionDuration: {
        '400': '400ms',
      },
      boxShadow: {
        'card': '0 1px 3px rgba(0,0,0,0.02), 0 1px 2px rgba(0,0,0,0.01)',
        'card-hover': '0 10px 30px rgba(0,0,0,0.04), 0 2px 8px rgba(0,0,0,0.02)',
        'elevated': '0 20px 50px rgba(0,0,0,0.06), 0 4px 12px rgba(0,0,0,0.03)',
        'sidebar': '1px 0 0 rgba(0,0,0,0.03)',
      },
    },
  },
  plugins: [],
}
