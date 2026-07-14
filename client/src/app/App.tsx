import { useEffect, useState } from 'react';
import { WorkbenchShell } from './shell/WorkbenchShell.tsx';
import { getInitialWorkbenchViewModel } from '../runtime/staticWorkbenchAdapter.tsx';
import { wailsWorkbenchAdapter } from '../runtime/wailsWorkbenchAdapter.ts';
import { AppThemeProvider } from '../theme/ThemeProvider.tsx';

export default function App() {
  const [viewModel, setViewModel] = useState(getInitialWorkbenchViewModel());

  useEffect(() => {
    let active = true;

    void wailsWorkbenchAdapter
      .loadInitialViewModel()
      .then((nextViewModel) => {
        if (active) {
          setViewModel(nextViewModel);
        }
      })
      .catch((error) => {
        console.error('[app] failed to load initial runtime view model', error);
      });

    return () => {
      active = false;
    };
  }, []);

  return (
    <AppThemeProvider appearance={viewModel.settings.appearance}>
      <WorkbenchShell adapter={wailsWorkbenchAdapter} viewModel={viewModel} />
    </AppThemeProvider>
  );
}
