package shell

import "unicode/utf8"

func normalizeCommandOutput(output string) string {
	if output == "" || utf8.ValidString(output) {
		return output
	}
	return decodeLocalOutput(output)
}
