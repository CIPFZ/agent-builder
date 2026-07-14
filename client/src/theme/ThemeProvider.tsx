import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { applyCSSVariables, toAntdTheme } from './adapters.ts';
import type { AppearanceSettings, EffectiveColorMode } from './contract.ts';
import { resolveTheme } from './registry.ts';

export function AppThemeProvider({ appearance, children }: { appearance: AppearanceSettings; children: ReactNode }) {
  const media = useMemo(() => window.matchMedia('(prefers-color-scheme: dark)'), []);
  const [systemMode, setSystemMode] = useState<EffectiveColorMode>(media.matches ? 'dark' : 'light');
  const effectiveMode = appearance.colorMode === 'system' ? systemMode : appearance.colorMode;
  const selectedTheme = resolveTheme(appearance.themeId);
  const tokens = selectedTheme.modes[effectiveMode];

  useEffect(() => {
    const update = (event: MediaQueryListEvent) => setSystemMode(event.matches ? 'dark' : 'light');
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }, [media]);

  useEffect(() => {
    applyCSSVariables(tokens);
    document.documentElement.dataset.colorMode = effectiveMode;
    document.documentElement.dataset.theme = selectedTheme.id;
    document.documentElement.style.colorScheme = effectiveMode;
    window.dispatchEvent(new CustomEvent('app-theme-change'));
  }, [effectiveMode, selectedTheme.id, tokens]);

  return <ConfigProvider locale={zhCN} theme={toAntdTheme(tokens, effectiveMode)}>{children}</ConfigProvider>;
}
