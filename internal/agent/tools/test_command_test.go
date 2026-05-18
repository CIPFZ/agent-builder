package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("CRUSH_TEST_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	args = args[1:]
	if len(args) == 0 {
		os.Exit(2)
	}

	switch args[0] {
	case "sleep":
		if len(args) != 2 {
			os.Exit(2)
		}
		ms, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Duration(ms) * time.Millisecond)
	case "slow-lines":
		for i := 1; i <= 5; i++ {
			fmt.Printf("line %d\n", i)
			time.Sleep(50 * time.Millisecond)
		}
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func helperCommand(args ...string) string {
	quoted := []string{shellQuote(filepath.ToSlash(os.Args[0])), "-test.run=TestHelperProcess", "--"}
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return "CRUSH_TEST_HELPER_PROCESS=1 " + strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
