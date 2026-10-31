package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpDoesNotStartApplication(t *testing.T) {
	t.Setenv("mode", "unknown")
	var output bytes.Buffer
	if err := run([]string{"-h"}, strings.NewReader(""), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "用法:") || !strings.Contains(output.String(), "mode") {
		t.Fatalf("启动帮助不完整：%s", output.String())
	}
}

func TestInvalidFlagsReturnChineseError(t *testing.T) {
	for _, args := range [][]string{{"-unknown"}, {"-dir"}} {
		var output bytes.Buffer
		err := run(args, strings.NewReader(""), &output)
		if err == nil || err.Error() != "启动参数无效，请使用 -h 查看用法" {
			t.Fatalf("期望中文参数错误，实际为 %v", err)
		}
	}
}
