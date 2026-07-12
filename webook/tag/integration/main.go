// Package integration 汇集 tag 服务的集成测试（真实 DB 装配见 setup 子包）。
//
// 本包命名文件（非测试）是该 test-only 目录的锚点，保证 `wire ./...` 能推导输出目录并跳过本无 injector 包；
// 删掉会让 wire 在 detectOutputDir 阶段报 "no files to derive output directory from" 退 1。详见 webook/CLAUDE.md「集成测试规范」。
package integration
