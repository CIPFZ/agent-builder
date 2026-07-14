import type { AppTheme } from '../../contract.ts';
import { defaultDarkTokens } from './dark.ts';
import { defaultLightTokens } from './light.ts';

export const defaultTheme: AppTheme = {
  id: 'builtin.default',
  name: 'Default',
  modes: { light: defaultLightTokens, dark: defaultDarkTokens },
};
