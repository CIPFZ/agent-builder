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
	for _, raw := range asSlice(payload["reinjectedRefs"]) {
		switch item := raw.(type) {
		case string:
			boundary.ReinjectedRefs = append(boundary.ReinjectedRefs, RuntimeReinjectedRef{
				ID:     item,
				Kind:   "legacy",
				Ref:    item,
				Status: compactStatusCompleted,
				Reason: "legacy_reinjected_ref",
			})
		case map[string]any:
			boundary.ReinjectedRefs = append(boundary.ReinjectedRefs, runtimeReinjectedRefFromPayload(item))
		}
	}
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

func runtimeReinjectedRefFromPayload(payload map[string]any) RuntimeReinjectedRef {
	return RuntimeReinjectedRef{
		ID:             stringFromMap(payload, "id"),
		Kind:           stringFromMap(payload, "kind"),
		Name:           stringFromMap(payload, "name"),
		Path:           stringFromMap(payload, "path"),
		URI:            stringFromMap(payload, "uri"),
		Ref:            stringFromMap(payload, "ref"),
		Status:         stringFromMap(payload, "status"),
		Reason:         stringFromMap(payload, "reason"),
		Error:          stringFromMap(payload, "error"),
		ContentSummary: stringFromMap(payload, "contentSummary"),
		TokenEstimate:  intFromMap(payload, "tokenEstimate"),
	}
}
