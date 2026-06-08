# GitHub Token 配置与 Release 说明更新

## 1. 创建 Token

1. 打开 https://github.com/settings/tokens
2. **Fine-grained token** 或 **Classic token**
3. 权限至少包含：`repo`（或目标仓库的 Contents: Read and write）

## 2. 本地设置环境变量

```bash
# Linux / macOS
export GITHUB_TOKEN=ghp_xxxxxxxx

# PowerShell
$env:GITHUB_TOKEN = "ghp_xxxxxxxx"
```

## 3. 更新 Release 说明

```bash
cd quota-dns-router-go
chmod +x scripts/update-github-release-body.sh

# 更新 v0.2.0
./scripts/update-github-release-body.sh v0.2.0 docs/releases/v0.2.0.md

# 更新 v0.2.1
./scripts/update-github-release-body.sh v0.2.1 docs/releases/v0.2.1.md
```

## 4. 验证

浏览器打开对应 Release 页面，确认 Description 已显示 Markdown 正文。

## 故障排查

| 现象 | 处理 |
|------|------|
| `401 Bad credentials` | Token 过期或权限不足 |
| `404 Not Found` | tag 尚未创建 Release |
| `python3: not found` | 安装 Python 3，或手动粘贴 `docs/releases/*.md` |
