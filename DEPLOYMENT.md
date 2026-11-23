# 共建工作台：本地部署与代码范围

本仓库来自 2026-11-23 的“共建工作台-脱敏源码”压缩包。专属主号、来源账号、固定令牌清单、历史扫描起点、密钥文件、分组名册、账本及计划任务未包含在源码中。

## 本地构建

要求 Go 1.26.8 或更新版本。在仓库根目录执行：

```powershell
go test ./...
go build -trimpath -buildvcs=false -o workbench.exe .
```

使用一个全新的专用数据目录启动。程序默认只监听 `127.0.0.1:8910`；地址占用时会向后寻找空闲端口。启动日志会打印一次性本地访问链接。

```powershell
.\workbench.exe -keys 'C:\path\to\private-data\keys.json' -no-open
```

首次运行时可在终端用 `-add master`、`-add member` 或 `-add derive` 导入账号。助记词和私钥只应输入到本机终端，切勿提交到 GitHub、粘贴到网页或写入部署脚本。此仓库的 `.gitignore` 排除常见数据文件，但不能代替上传前检查。

## 结构

- `main.go` / `server.go`：本地 HTTP 服务、会话访问令牌和 API。
- `keystore*.go` / `secure_file*.go`：加密密钥文件、解锁及文件权限。
- `chain.go` / `squad*.go` / `runner.go` / `batch.go`：RPC、Squad 合约、交易执行与批次。
- `dashboard*.go` / `public/`：链上统计与工作台页面。
- `福泽模拟/`：独立只读模拟脚本；需要另行配置专属地址和任务。
- `扩组核算/`：独立工作簿生成与核算脚本；需要另行配置名册、账本和依赖。

## 运行边界

默认 RPC 指向 Anubis 主网，链 ID 为 6714。导入并解锁真实钱包后，部署、授权、打局、转账和归集操作会产生真实链上交易。空数据目录只能验证界面与连接，无法恢复原操盘者的账户或专属报表。固定令牌清单操作在脱敏配置下会直接拒绝执行。
