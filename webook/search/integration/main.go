// Package integration 汇集 search 服务的集成测试（真实 ES 装配见 setup 子包）。
//
// main.go 与 main_test.go 成对：后者放 TestMain，本文件是该 test-only 目录唯一的非测试锚点，
// 令 `wire ./...` 能推导输出目录、跳过本无 injector 包（否则 detectOutputDir 阶段报
// "no files to derive output directory from" 令整条命令退 1）。详见 webook/CLAUDE.md「集成测试规范」。
package integration
