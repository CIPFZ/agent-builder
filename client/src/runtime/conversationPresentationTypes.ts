export interface RuntimeExplorationCount { kind: string; count: number; failed?: number }
export interface RuntimeExplorationSummary { status: string; toolCounts?: RuntimeExplorationCount[]; toolTotal?: number; failedCount?: number; subagentCount?: number; thinkingMs?: number; elapsedMs?: number }
export interface RuntimeCompactInfo { trigger?: string; status?: string; preTokens?: number; postTokens?: number; summarizedCount?: number; summaryMessageId?: string; summaryText?: string; error?: string }
