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
    components: {
      Button: {
        defaultHoverColor: tokens.textPrimary,
        defaultHoverBorderColor: tokens.borderDefault,
        defaultActiveColor: tokens.textPrimary,
        defaultActiveBorderColor: tokens.textTertiary,
      },
      Input: {
        activeBorderColor: tokens.textPrimary,
        hoverBorderColor: tokens.textSecondary,
        activeShadow: `0 0 0 2px ${tokens.focusRing}`,
      },
      InputNumber: {
        activeBorderColor: tokens.textPrimary,
        hoverBorderColor: tokens.textSecondary,
        activeShadow: `0 0 0 2px ${tokens.focusRing}`,
      },
      Select: {
        activeBorderColor: tokens.textPrimary,
        activeOutlineColor: tokens.focusRing,
        hoverBorderColor: tokens.textSecondary,
        optionActiveBg: tokens.surfaceHover,
        optionSelectedBg: tokens.surfaceActive,
        optionSelectedColor: tokens.textPrimary,
      },
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
