package shell

import (
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func decodeLocalOutput(output string) string {
	decoded, err := simplifiedchinese.GBK.NewDecoder().String(output)
	if err != nil {
		return strings.ToValidUTF8(output, "\uFFFD")
	}
	return decoded
}
