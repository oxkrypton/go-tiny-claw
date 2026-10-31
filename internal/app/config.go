package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

const (
	modeCLI    = "cli"
	modeFeishu = "feishu"
)

// Options 是调用方传入的启动选项，不依赖具体的参数解析方式。
type Options struct {
	WorkDir       string
	SessionID     string
	InitialPrompt string
}

type config struct {
	mode          string
	workDir       string
	sessionID     string
	initialPrompt string
}

func loadConfig(options Options) (config, error) {
	mode, err := loadMode()
	if err != nil {
		return config{}, err
	}
	if mode == modeFeishu && (strings.TrimSpace(os.Getenv("FEISHU_APP_ID")) == "" || strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")) == "") {
		return config{}, errors.New("飞书模式需要设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
	}
	if strings.TrimSpace(os.Getenv("API_KEY")) == "" {
		return config{}, errors.New("请设置 API_KEY 环境变量")
	}
	if strings.TrimSpace(os.Getenv("baseURL")) == "" {
		return config{}, errors.New("请设置 baseURL 环境变量")
	}

	workDir := options.WorkDir
	if workDir == "" {
		workDir = "."
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return config{}, errors.New("解析工作区路径失败，请检查工作目录")
	}
	sessionID := options.SessionID
	if sessionID == "" {
		sessionID = "cli_default_session"
	}
	return config{
		mode: mode, workDir: workDir, sessionID: sessionID,
		initialPrompt: options.InitialPrompt,
	}, nil
}

// loadMode 从当前目录的 .env 加载配置，已有环境变量优先。
func loadMode() (string, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("读取 .env 配置失败，请检查文件权限和配置格式")
	}

	mode := strings.ToLower(strings.TrimSpace(os.Getenv("mode")))
	switch mode {
	case "", "cli":
		return "cli", nil
	case "feishu":
		return "feishu", nil
	default:
		return "", errors.New("mode 配置无效，请设置为 cli 或 feishu")
	}
}
