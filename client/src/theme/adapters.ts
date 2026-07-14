import type { ThemeConfig } from 'antd';
import { theme as antdTheme } from 'antd';
import type { AppThemeTokens, EffectiveColorMode } from './contract.ts';

export function toAntdTheme(tokens: AppThemeTokens, mode: EffectiveColorMode): ThemeConfig {
  return {
    algorithm: mode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      colorPrimary: tokens.colorPrimary, colorSuccess: tokens.colorSuccess, colorWarning: tokens.colorWarning,
      colorError: tokens.colorError, colorBgBase: tokens.surfaceCanvas, colorBgContainer: tokens.surfacePanel,
      colorBgElevated: tokens.surfaceElevated, colorText: tokens.textPrimary, colorTextSecondary: tokens.textSecondary,
      colorTextTertiary: tokens.textTertiary, colorBorder: tokens.borderDefault, colorBorderSecondary: tokens.borderSubtle,
    },
  };
}

export function applyCSSVariables(tokens: AppThemeTokens) {
  const root = document.documentElement;
  for (const [key, value] of Object.entries(tokens)) {
    const cssName = key.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
    root.style.setProperty(`--app-${cssName}`, value);
  }
}
