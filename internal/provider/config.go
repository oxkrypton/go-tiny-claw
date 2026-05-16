package provider

import (
	"os"

	"github.com/joho/godotenv"
)

func loadConfig() (apiKey, baseURL string) {
	_ = godotenv.Load()

	apiKey = os.Getenv("API_KEY")
	if apiKey == "" {
		panic("请设置 API_KEY 环境变量")
	}
	baseURL = os.Getenv("baseURL")
	if baseURL == "" {
		panic("请设置 baseURL 环境变量")
	}
	return
}
