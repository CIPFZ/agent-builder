package runtime

func runtimeCompactBoundaryFromPayload(payload map[string]any) RuntimeCompactBoundary {
	boundary := RuntimeCompactBoundary{
		ID:          stringFromMap(payload, "id"),
		SessionID:   stringFromMap(payload, "sessionId"),
		TurnID:      stringFromMap(payload, "turnId"),
		Kind:        stringFromMap(payload, "kind"),
		Trigger:     stringFromMap(payload, "trigger"),
		Status:      stringFromMap(payload, "status"),
		SummaryRef:  stringFromMap(payload, "summaryRef"),
		Error:       stringFromMap(payload, "error"),
		CreatedAt:   int64(intFromMap(payload, "createdAt")),
		CompletedAt: int64(intFromMap(payload, "completedAt")),
	}
	if before, ok := payload["budgetBefore"].(map[string]any); ok {
		boundary.BudgetBefore = runtimeBudgetReportFromPayload(before)
	}
	if after, ok := payload["budgetAfter"].(map[string]any); ok {
		boundary.BudgetAfter = runtimeBudgetReportFromPayload(after)
	}
	boundary.MessageRefs = stringSliceFromMap(payload, "messageRefs")
	boundary.ReinjectedRefs = stringSliceFromMap(payload, "reinjectedRefs")
	rawRefs, ok := payload["toolCallRefs"].([]any)
	if ok {
		for _, raw := range rawRefs {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			boundary.ToolCallRefs = append(boundary.ToolCallRefs, RuntimeCompactToolCallRef{
				ToolCallID:      stringFromMap(item, "toolCallId"),
				Name:            stringFromMap(item, "name"),
				Ref:             stringFromMap(item, "ref"),
				EstimatedTokens: intFromMap(item, "estimatedTokens"),
				Replacement:     stringFromMap(item, "replacement"),
				Preserved:       boolFromMap(item, "preserved"),
				Reason:          stringFromMap(item, "reason"),
			})
		}
	}
	return boundary
}
