package mcpclient

import "strings"

// sanitizeName 把 MCP server/tool 名称转成模型函数调用可接受的安全名称。
// 规则：只保留字母、数字、下划线；其他字符转下划线；全部小写。
func sanitizeName(name string) string {
	name = strings.ToLower(name)

	var b strings.Builder
	prevUnderscore := false

	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'

		if ok {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}

		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}

	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unnamed"
	}
	return out
}
