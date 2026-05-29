package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func main() {
	inputFile := "mock_log.txt"
	linesPerFile := 100

	// 构建输出目录名：split_20060102150405
	dirName := "split_" + time.Now().Format("20060102150405")

	// 创建目录
	err := os.Mkdir(dirName, 0755)
	if err != nil {
		fmt.Printf("创建目录失败：%v\n", err)
		os.Exit(1)
	}

	// 打开输入文件
	f, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("打开文件失败：%v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	fileIndex := 0
	lineCount := 0

	var currentFile *os.File
	var writer *bufio.Writer

	for scanner.Scan() {
		// 如果还没打开文件，或者已经写了 100 行，新建一个
		if lineCount == 0 {
			if currentFile != nil {
				writer.Flush()
				currentFile.Close()
			}
			fileName := "log_" + strconv.Itoa(fileIndex) + ".txt"
			filePath := filepath.Join(dirName, fileName)
			currentFile, err = os.Create(filePath)
			if err != nil {
				fmt.Printf("创建文件 %s 失败：%v\n", filePath, err)
				os.Exit(1)
			}
			writer = bufio.NewWriter(currentFile)
			fileIndex++
		}

		// 写入当前行
		_, err := writer.WriteString(scanner.Text() + "\n")
		if err != nil {
			fmt.Printf("写入文件失败：%v\n", err)
			os.Exit(1)
		}

		lineCount++
		if lineCount >= linesPerFile {
			lineCount = 0
		}
	}

	// 扫尾
	if writer != nil {
		writer.Flush()
	}
	if currentFile != nil {
		currentFile.Close()
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("读取文件失败：%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("拆分完成！共生成 %d 个文件，保存在 %s 目录下。\n", fileIndex, dirName)
}
