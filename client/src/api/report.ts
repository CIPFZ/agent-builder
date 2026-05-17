import type { EvidenceItem, Recommendation, SopFixture, SshTarget } from '../types/runtime'

export type ReportRequest = {
  userGoal: string
  sop: SopFixture
  target: SshTarget
  evidence: EvidenceItem[]
}

export type GeneratedReport = Recommendation & {
  provider: 'deepseek' | 'fallback'
  summary?: string
  findings?: string[]
}

export async function generateTroubleshootingReport(request: ReportRequest): Promise<GeneratedReport> {
  const response = await fetch('http://127.0.0.1:4177/api/deepseek/report', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(request),
  })

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new Error(payload.message || `Report request failed with ${response.status}`)
  }

  return response.json()
}
