package main

import (
	"fmt"
	"log"
)

func main() {
	fmt.Println("go-tiny-claw启动:")

	//todo1:初始化模型 provider (大脑)
	//provider:=provider.NewClaudeProvider(...)

	//todo2:初始化 Tool Registry (手脚)

	//todo3:初始化上下文管理器(内存管理器)

	//todo4:组装并启动核心 Engine (操作系统心脏)

	// fmt.Println("开始执行任务...")
	// err := engine.Run("帮我检查一下当前目录下的文件并输出一个 README.md 大纲")
	// if err != nil {
	// log.Fatalf("引擎运行崩溃: %v", err)
	// }

	log.Println("初步搭建框架完成, 待各模块注入")
}
