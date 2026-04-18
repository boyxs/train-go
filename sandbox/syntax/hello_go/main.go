package main

import (
	"fmt"
)

func main() {
	fmt.Println("hello world!")
	hello()
	test(false, false)
}

// go mod init hello
// go run main.go
// go build -o main.exe
// 以下指令可以解决外部依赖引找不到的问题
// go run .
// go build .
