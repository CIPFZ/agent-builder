package tools

import "os"

func toolSearchEnabledOptimistic() bool {
	if envTruthy(os.Getenv("CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS")) {
		return false
	}
	mode := getToolSearchMode()
	if mode == toolSearchModeStandard {
		return false
	}
	return true
}

type toolSearchMode string

const (
	toolSearchModeEnabled  toolSearchMode = "tst"
	toolSearchModeAuto     toolSearchMode = "tst-auto"
	toolSearchModeStandard toolSearchMode = "standard"
)

func getToolSearchMode() toolSearchMode {
	value := os.Getenv("ENABLE_TOOL_SEARCH")
	if value == "" {
		return toolSearchModeEnabled
	}
	if value == "auto" {
		return toolSearchModeAuto
	}
	if autoPercent, ok := parseAutoPercentage(value); ok {
		switch autoPercent {
		case 0:
			return toolSearchModeEnabled
		case 100:
			return toolSearchModeStandard
		default:
			return toolSearchModeAuto
		}
	}
	if envFalsy(value) {
		return toolSearchModeStandard
	}
	return toolSearchModeEnabled
}

func parseAutoPercentage(value string) (int, bool) {
	if len(value) <= len("auto:") || value[:len("auto:")] != "auto:" {
		return 0, false
	}
	percent := 0
	for _, ch := range value[len("auto:"):] {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		percent = percent*10 + int(ch-'0')
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return percent, true
}

func envTruthy(value string) bool {
	switch value {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	}
	return false
}

func envFalsy(value string) bool {
	switch value {
	case "0", "false", "FALSE", "False", "no", "NO", "off", "OFF":
		return true
	default:
		return false
	}
}
