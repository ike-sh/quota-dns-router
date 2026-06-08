# 使用 GitHub CLI 编辑 Release

## Windows 安装 gh

### 方式 1：winget（推荐）

```powershell
winget install --id GitHub.cli
```

安装后**重新打开终端**，验证：

```powershell
gh --version
```

### 方式 2：Scoop

```powershell
scoop install gh
```

### 方式 3：官方安装包

从 [GitHub CLI Releases](https://github.com/cli/cli/releases) 下载 Windows `.msi` 安装。

## 登录

```powershell
gh auth login
```

按提示选择 GitHub.com → HTTPS → 浏览器授权。

## 编辑 v0.2.3 Release 说明

在项目根目录执行：

```powershell
cd d:\code\quota-dns-router-go
gh release edit v0.2.3 --notes-file docs/releases/v0.2.3-github-release-body.md
```

若 Release 尚未由 CI 创建，可先查看：

```powershell
gh release list
gh release view v0.2.3
```

## 手动创建 Release（CI 未触发时）

```powershell
git tag -a v0.2.3 -m "v0.2.3"
git push origin v0.2.3
```

等待 [Actions → release](https://github.com/ike-sh/quota-dns-router/actions) 完成后，再执行 `gh release edit`。

## 上传补充资产（可选）

CI 已上传二进制时通常无需手动上传。若需要：

```powershell
gh release upload v0.2.3 dist/*.tar.gz dist/SHA256SUMS
```
