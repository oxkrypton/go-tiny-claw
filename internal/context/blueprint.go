package context

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// BlueprintLoader scans .claw/blueprint/*.md and injects a short index into the
// system prompt. The agent reads full blueprint files on-demand via read_file
// when it actually needs to work in a given directory — avoiding per-turn
// token waste on content the agent may never touch.
type BlueprintLoader struct {
	workDir string
}

func NewBlueprintLoader(workDir string) *BlueprintLoader {
	return &BlueprintLoader{workDir: workDir}
}

// LoadIndex returns a compact index of available blueprint files.
// Returns empty string if .claw/blueprint/ doesn't exist or is empty.
func (b *BlueprintLoader) LoadIndex() string {
	blueprintDir := filepath.Join(b.workDir, ".claw", "blueprint")

	if _, err := os.Stat(blueprintDir); os.IsNotExist(err) {
		return ""
	}

	var files []string
	_ = filepath.WalkDir(blueprintDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		files = append(files, path)
		return nil
	})

	if len(files) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("\n\n# 架构蓝图索引 (Blueprint)\n")
	builder.WriteString(".claw/blueprint/ 下存放了关键目录的设计指南。当你需要操作某个目录下的文件时，先用 read_file 读取对应的 blueprint 了解约定和陷阱：\n\n")

	for _, path := range files {
		rel, _ := filepath.Rel(blueprintDir, path)
		dir := strings.TrimSuffix(rel, ".md")
		desc := extractFirstHeading(path)
		builder.WriteString(fmt.Sprintf("- `%s/` → .claw/blueprint/%s  %s\n", dir, rel, desc))
	}

	return builder.String()
}

// extractFirstHeading reads the first "# " line from a markdown file.
func extractFirstHeading(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 5)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return "— " + strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}
