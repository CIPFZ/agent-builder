import { WorkbenchShell } from './shell/WorkbenchShell.tsx';
import { getInitialWorkbenchViewModel } from '../runtime/staticWorkbenchAdapter.tsx';

export default function App() {
  const viewModel = getInitialWorkbenchViewModel();

  return <WorkbenchShell viewModel={viewModel} />;
}
