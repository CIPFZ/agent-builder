import type { AppTheme } from './contract.ts';
import { defaultTheme } from './themes/default/index.ts';

const themes = new Map<string, AppTheme>([[defaultTheme.id, defaultTheme]]);

export function registerTheme(theme: AppTheme) {
  themes.set(theme.id, theme);
}

export function resolveTheme(themeId: string) {
  return themes.get(themeId) ?? defaultTheme;
}
