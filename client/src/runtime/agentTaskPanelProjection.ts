import { deriveAgentActivitySummary, deriveAgentTeamPresentations, type AgentActivitySummary, type AgentTeamPresentation } from './conversationPresentationModel.ts';
import type { AgentTaskViewModel } from './workbenchTypes.ts';

export interface AgentTaskPanelProjection { summary: AgentActivitySummary; teams: AgentTeamPresentation[]; independent: AgentTaskViewModel[] }

export function projectAgentTaskPanel(sessionId: string, tasks: AgentTaskViewModel[]): AgentTaskPanelProjection {
  const owned = tasks.filter((task) => task.parentSessionId === sessionId);
  return { summary: deriveAgentActivitySummary(sessionId, owned), teams: deriveAgentTeamPresentations(sessionId, owned), independent: owned.filter((task) => !task.teamId) };
}
