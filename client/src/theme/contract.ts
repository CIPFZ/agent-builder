export type ColorMode = 'system' | 'light' | 'dark';
export type EffectiveColorMode = Exclude<ColorMode, 'system'>;

export interface AppearanceSettings {
  colorMode: ColorMode;
  themeId: string;
}

export interface AppThemeTokens {
  colorPrimary: string;
  colorSuccess: string;
  colorWarning: string;
  colorError: string;
  surfaceCanvas: string;
  surfacePanel: string;
  surfaceElevated: string;
  surfaceMuted: string;
  surfaceHover: string;
  surfaceActive: string;
  textPrimary: string;
  textSecondary: string;
  textTertiary: string;
  borderDefault: string;
  borderSubtle: string;
  focusRing: string;
  shadowColor: string;
  syntaxAdded: string;
  syntaxKeyword: string;
  syntaxString: string;
  syntaxLink: string;
  syntaxRemoved: string;
}

export interface AppTheme {
  id: string;
  name: string;
  modes: Record<EffectiveColorMode, AppThemeTokens>;
}
