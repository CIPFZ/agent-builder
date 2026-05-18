package shell

import "testing"

func TestNormalizeCommandOutputKeepsUTF8(t *testing.T) {
	t.Parallel()

	input := "Ping www.baidu.com [198.18.0.184] 具有 32 字节的数据:\n"
	if got := normalizeCommandOutput(input); got != input {
		t.Fatalf("normalizeCommandOutput() = %q, want %q", got, input)
	}
}
