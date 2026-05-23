package runtime

import "slices"

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
		if !skill.Enabled || skill.State == capabilityStateDisabled {
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
