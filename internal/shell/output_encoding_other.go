//go:build !windows

package shell

import "strings"

func decodeLocalOutput(output string) string {
	return strings.ToValidUTF8(output, "\uFFFD")
}
