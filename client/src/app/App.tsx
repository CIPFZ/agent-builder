import { useEffect, useState } from 'react';
import { WorkbenchShell } from './shell/WorkbenchShell.tsx';
import { getInitialWorkbenchViewModel } from '../runtime/staticWorkbenchAdapter.tsx';
import { wailsWorkbenchAdapter } from '../runtime/wailsWorkbenchAdapter.ts';

export default function App() {
  const [viewModel, setViewModel] = useState(getInitialWorkbenchViewModel());
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let active = true;

    void wailsWorkbenchAdapter.loadInitialViewModel().then((nextViewModel) => {
      if (active) {
        setViewModel(nextViewModel);
        setLoaded(true);
      }
    });

    return () => {
      active = false;
    };
  }, []);

  return <WorkbenchShell key={loaded ? 'runtime' : 'booting'} adapter={wailsWorkbenchAdapter} viewModel={viewModel} />;
}
