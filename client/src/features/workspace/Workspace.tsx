import type { WorkbenchViewModel } from '../../runtime/workbenchTypes.ts';
import { Composer } from '../composer/Composer.tsx';
import styles from './Workspace.module.css';

interface WorkspaceProps {
  viewModel: WorkbenchViewModel;
}

export function Workspace({ viewModel }: WorkspaceProps) {
  const title =
    viewModel.mode === 'project'
      ? `我们应该在 ${viewModel.currentProject.name} 中构建什么？`
      : '我们该做什么？';

  return (
    <section className={styles.workspace} data-mode={viewModel.mode}>
      <div className={styles.content}>
        <h1 className={styles.title}>{title}</h1>
        <Composer
          composer={viewModel.composer}
          project={viewModel.currentProject}
          showProjectContext={viewModel.mode === 'project'}
        />
      </div>
    </section>
  );
}
