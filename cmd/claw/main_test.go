package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMode(t *testing.T) {
	tests := []struct {
		name     string
		dotenv   string
		noFile   bool
		env      string
		override bool
		want     string
		wantErr  string
	}{
		{name: "无配置默认命令行", noFile: true, want: "cli"},
		{name: "未指定模式默认命令行", dotenv: "# 未指定模式\n", want: "cli"},
		{name: "空模式默认命令行", dotenv: "mode=\n", want: "cli"},
		{name: "命令行模式", dotenv: "mode=cli\n", want: "cli"},
		{name: "飞书模式", dotenv: "mode=feishu\n", want: "feishu"},
		{name: "兼容大小写和空格", dotenv: "mode=\" FeIsHu \"\n", want: "feishu"},
		{name: "已有环境变量优先", dotenv: "mode=feishu\n", env: "cli", override: true, want: "cli"},
		{name: "环境变量启用飞书", noFile: true, env: "feishu", override: true, want: "feishu"},
		{name: "未知模式", dotenv: "mode=unknown\n", wantErr: "mode 配置无效，请设置为 cli 或 feishu"},
		{name: "配置格式错误", dotenv: "mode=\"feishu\n", wantErr: "读取 .env 配置失败，请检查文件权限和配置格式"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("mode", tt.env)
			if !tt.override {
				if err := os.Unsetenv("mode"); err != nil {
					t.Fatal(err)
				}
			}
			if !tt.noFile {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tt.dotenv), 0600); err != nil {
					t.Fatal(err)
				}
			}

			got, err := loadMode()
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("期望错误 %q，实际为 %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("期望模式 %q，实际为 %q", tt.want, got)
			}
		})
	}
}
