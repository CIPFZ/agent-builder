package runtime

import (
	"slices"
	"time"

	"github.com/CIPFZ/agent-builder/internal/runtimeapi"
)

func nowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func runtimeTurnSkillSummary(skills []RuntimeSkill, policyMode string) RuntimeTurnSkillSummary {
	summary := RuntimeTurnSkillSummary{
		AvailableCount: len(skills),
		PolicyMode:     policyMode,
		PolicyRisk:     "read",
		PolicyReason:   "Skill activation metadata is evaluated at the runtime boundary; allowed_tools metadata is preserved but does not expand permissions.",
	}
	for _, skill := range skills {
		item := RuntimeSkillTurnItem{
			Name:          skill.Name,
			CapabilityID:  firstNonEmpty(skill.CapabilityID, "skill:"+skill.Name),
			Builtin:       skill.Builtin,
			Path:          skill.Path,
			SkillFilePath: skill.SkillFilePath,
			State:         skill.State,
			Reason:        firstNonEmpty(skill.Activation.Reason, skill.Reason),
			Error:         skill.Error,
			AllowedTools:  append([]string(nil), skill.AllowedTools...),
		}
		if skill.Path != "" && !slices.Contains(summary.SourcePaths, skill.Path) {
			summary.SourcePaths = append(summary.SourcePaths, skill.Path)
		}
		if skill.State == capabilityStateFailed || skill.Error != "" {
			summary.Failed = append(summary.Failed, item)
			continue
		}
		if !skill.Enabled || skill.State == capabilityStateDisabled || (skill.Activation.Available && !skill.Activation.Included) {
			if item.Reason == "" {
				item.Reason = "excluded by disabled config"
			}
			summary.Excluded = append(summary.Excluded, item)
			continue
		}
		summary.Available = append(summary.Available, item)
		item.State = capabilityStateLoaded
		if item.Reason == "" {
			item.Reason = "runtime included in prompt"
		}
		summary.Activated = append(summary.Activated, item)
	}
	return summary
}

func (r *runtimeService) recordTurnSkillActivation(sessionID, turnID string, summary RuntimeTurnSkillSummary) {
	for _, item := range summary.Activated {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventSkillActivationAllowed,
			CreatedAt: nowRFC3339Nano(),
			SessionID: sessionID,
			TurnID:    turnID,
			Payload: map[string]any{
				"name":          item.Name,
				"capability_id": item.CapabilityID,
				"state":         item.State,
				"reason":        firstNonEmpty(item.Reason, "runtime activation allowed"),
				"allowed_tools": item.AllowedTools,
				"summary":       item.Name,
			},
		})
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventSkillContextInjected,
			CreatedAt: nowRFC3339Nano(),
			SessionID: sessionID,
			TurnID:    turnID,
			Payload: map[string]any{
				"name":          item.Name,
				"capability_id": item.CapabilityID,
				"reason":        firstNonEmpty(item.Reason, "runtime context injected"),
				"summary":       item.Name,
			},
		})
		r.writeAudit(auditEntry{
			RequestID:        turnID,
			Event:            "skill_activation_allowed",
			Timestamp:        nowRFC3339Nano(),
			SessionID:        sessionID,
			CapabilityID:     item.CapabilityID,
			CapabilityKind:   "skill",
			CapabilitySource: item.Path,
			CapabilityState:  item.State,
			CapabilityReason: item.Reason,
			Extra: map[string]any{
				"name":          item.Name,
				"allowed_tools": item.AllowedTools,
			},
		})
	}
	for _, item := range summary.Excluded {
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventSkillActivationDenied,
			CreatedAt: nowRFC3339Nano(),
			SessionID: sessionID,
			TurnID:    turnID,
			Payload: map[string]any{
				"name":          item.Name,
				"capability_id": item.CapabilityID,
				"state":         item.State,
				"reason":        firstNonEmpty(item.Reason, "skill activation excluded"),
				"allowed_tools": item.AllowedTools,
				"summary":       item.Name,
			},
		})
		r.storeRuntimeEvent(runtimeapi.Event{
			ID:        newRuntimeEventID(),
			Type:      runtimeapi.EventSkillContextOmitted,
			CreatedAt: nowRFC3339Nano(),
			SessionID: sessionID,
			TurnID:    turnID,
			Payload: map[string]any{
				"name":          item.Name,
				"capability_id": item.CapabilityID,
				"reason":        firstNonEmpty(item.Reason, "skill context omitted"),
				"summary":       item.Name,
			},
		})
		r.writeAudit(auditEntry{
			RequestID:        turnID,
			Event:            "skill_activation_denied",
			Timestamp:        nowRFC3339Nano(),
			SessionID:        sessionID,
			CapabilityID:     item.CapabilityID,
			CapabilityKind:   "skill",
			CapabilitySource: item.Path,
			CapabilityState:  item.State,
			CapabilityReason: item.Reason,
			Extra: map[string]any{
				"name":          item.Name,
				"allowed_tools": item.AllowedTools,
			},
		})
	}
}
