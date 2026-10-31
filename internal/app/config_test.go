package app

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

func configureTestEnvironment(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("mode", "cli")
	t.Setenv("API_KEY", "local-test")
	t.Setenv("baseURL", "http://127.0.0.1:1/v1")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	return dir
}

func TestLoadConfigOptions(t *testing.T) {
	dir := configureTestEnvironment(t)
	cfg, err := loadConfig(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.workDir != dir || cfg.sessionID != "cli_default_session" || cfg.mode != modeCLI {
		t.Fatalf("启动默认值不正确：%+v", cfg)
	}

	cfg, err = loadConfig(Options{WorkDir: "workspace", SessionID: "custom", InitialPrompt: "你好"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.workDir != filepath.Join(dir, "workspace") || cfg.sessionID != "custom" || cfg.initialPrompt != "你好" {
		t.Fatalf("启动选项未正确传递：%+v", cfg)
	}
}

func TestLoadConfigRejectsMissingCredentials(t *testing.T) {
	for _, tt := range []struct {
		name, mode, key, baseURL, appID, appSecret, wantErr string
	}{
		{name: "缺少模型密钥", mode: "cli", baseURL: "http://localhost/v1", wantErr: "请设置 API_KEY 环境变量"},
		{name: "缺少模型地址", mode: "cli", key: "test", wantErr: "请设置 baseURL 环境变量"},
		{name: "缺少飞书密钥", mode: "feishu", key: "test", baseURL: "http://localhost/v1", appID: "test", wantErr: "飞书模式需要设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET"},
		{name: "缺少飞书应用编号", mode: "feishu", key: "test", baseURL: "http://localhost/v1", appSecret: "test", wantErr: "飞书模式需要设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET"},
		{name: "飞书配置完整", mode: "feishu", key: "test", baseURL: "http://localhost/v1", appID: "test", appSecret: "test"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configureTestEnvironment(t)
			t.Setenv("mode", tt.mode)
			t.Setenv("API_KEY", tt.key)
			t.Setenv("baseURL", tt.baseURL)
			t.Setenv("FEISHU_APP_ID", tt.appID)
			t.Setenv("FEISHU_APP_SECRET", tt.appSecret)
			cfg, err := loadConfig(Options{})
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("期望错误 %q，实际为 %v", tt.wantErr, err)
				}
			} else if err != nil || cfg.mode != modeFeishu {
				t.Fatalf("完整飞书配置应可启动，实际模式 %q，错误 %v", cfg.mode, err)
			}
		})
	}
}
