import type { GlobalThemeOverrides } from 'naive-ui'

export const brand = {
  primary: '#2B6CF6',
  primaryHover: '#4A82F8',
  primaryPressed: '#1E56D6',
  primarySoft: '#EDF3FF',
  green: '#16A34A',
  red: '#DC2626',
  amber: '#F59E0B',
  slate: '#64748B',
  ink: '#0F172A',
  border: '#E6EAF2',
  canvas: '#F5F7FA',
}

export const darkThemeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#5b8ef7',
    primaryColorHover: '#7aa6ff',
    primaryColorPressed: '#4a78d6',
    primaryColorSuppl: '#7aa6ff',
    successColor: '#34d399',
    errorColor: '#f87171',
    warningColor: '#fbbf24',
    infoColor: '#5b8ef7',
    textColorBase: '#e8eef8',
    textColor1: '#e8eef8',
    textColor2: '#c3cfdf',
    textColor3: '#94a3b8',
    borderColor: '#263243',
    dividerColor: '#202b3a',
    bodyColor: '#0d131c',
    cardColor: '#151d28',
    modalColor: '#151d28',
    popoverColor: '#19222f',
    tableColor: '#151d28',
    tableHeaderColor: '#111821',
    inputColor: '#111821',
    actionColor: '#111821',
    hoverColor: '#1a2432',
    pressedColor: '#202c3d',
    placeholderColor: '#6f8098',
    borderRadius: '10px',
    borderRadiusSmall: '8px',
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
    fontFamilyMono: '"JetBrains Mono", "SFMono-Regular", Consolas, "Liberation Mono", monospace',
  },
  Button: {
    borderRadiusMedium: '9px',
    borderRadiusSmall: '7px',
    fontWeight: '500',
    heightMedium: '36px',
    heightLarge: '42px',
    paddingMedium: '0 16px',
    paddingLarge: '0 20px',
  },
  Input: {
    borderRadius: '9px',
    heightMedium: '38px',
    color: '#111821',
    colorFocus: '#151d28',
    border: '1px solid #2a3749',
    borderHover: '1px solid #3b4f6b',
    borderFocus: '1px solid #5b8ef7',
    boxShadowFocus: '0 0 0 3px rgba(91,142,247,.18)',
  },
  Select: {
    peers: {
      InternalSelection: {
        borderRadius: '9px',
        heightMedium: '38px',
        color: '#111821',
        colorActive: '#151d28',
        border: '1px solid #2a3749',
        borderHover: '1px solid #3b4f6b',
        borderActive: '1px solid #5b8ef7',
        boxShadowActive: '0 0 0 3px rgba(91,142,247,.18)',
      },
    },
  },
  Modal: {
    borderRadius: '16px',
  },
  Drawer: {
    borderRadius: '16px',
  },
  Tag: {
    borderRadius: '999px',
  },
  Switch: {
    railColorActive: '#5b8ef7',
  },
  Form: {
    labelFontWeight: '500',
    labelTextColor: '#c3cfdf',
  },
  Pagination: {
    itemBorderRadius: '8px',
  },
}

export const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: brand.primary,
    primaryColorHover: brand.primaryHover,
    primaryColorPressed: brand.primaryPressed,
    primaryColorSuppl: brand.primaryHover,
    successColor: brand.green,
    errorColor: brand.red,
    warningColor: brand.amber,
    infoColor: brand.primary,
    textColorBase: brand.ink,
    textColor1: '#0F172A',
    textColor2: '#334155',
    textColor3: '#64748B',
    borderColor: brand.border,
    dividerColor: '#EDF0F5',
    borderRadius: '10px',
    borderRadiusSmall: '8px',
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
    fontFamilyMono: '"JetBrains Mono", "SFMono-Regular", Consolas, "Liberation Mono", monospace',
  },
  Button: {
    borderRadiusMedium: '9px',
    borderRadiusSmall: '7px',
    fontWeight: '500',
    heightMedium: '36px',
    heightLarge: '42px',
    paddingMedium: '0 16px',
    paddingLarge: '0 20px',
  },
  Input: {
    borderRadius: '9px',
    heightMedium: '38px',
    color: '#FBFCFF',
    colorFocus: '#FFFFFF',
    border: '1px solid #E3E8F1',
    borderHover: '1px solid #A9C1FA',
    borderFocus: '1px solid #2B6CF6',
    boxShadowFocus: '0 0 0 3px rgba(43,108,246,.10)',
  },
  Select: {
    peers: {
      InternalSelection: {
        borderRadius: '9px',
        heightMedium: '38px',
        color: '#FBFCFF',
        colorActive: '#FFFFFF',
        border: '1px solid #E3E8F1',
        borderHover: '1px solid #A9C1FA',
        borderActive: '1px solid #2B6CF6',
        boxShadowActive: '0 0 0 3px rgba(43,108,246,.10)',
      },
    },
  },
  Modal: {
    borderRadius: '16px',
  },
  Drawer: {
    borderRadius: '16px',
  },
  Tag: {
    borderRadius: '999px',
  },
  Switch: {
    railColorActive: brand.primary,
  },
  Form: {
    labelFontWeight: '500',
    labelTextColor: '#334155',
  },
  Pagination: {
    itemBorderRadius: '8px',
  },
}
