package main

import (
	"fmt"
	"testing"
)

func TestMain(t *testing.T) {
	fmt.Println(parseClientSpec("name=xxx,cmd='command',env='KEY=val'")) // 测试代码
}
