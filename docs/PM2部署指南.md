# FT-API PM2 部署指南

> **版本**: 1.0.0
> **更新日期**: 2026-01-28

---

## 概述

本文档描述如何将 FT-API 从 Docker 部署迁移到 PM2 部署，实现更快速的代码更新流程。

### 部署架构

```
本地开发 → Git Push → GitHub → 服务器 Git Pull → Go Build → PM2 Restart
```

### 优势

| 对比项 | Docker 部署 | PM2 部署 |
|--------|------------|----------|
| 更新速度 | 慢（需要重建镜像） | 快（直接编译） |
| 资源占用 | 较高 | 较低 |
| 调试便利性 | 一般 | 好（直接查看日志） |
| 回滚速度 | 慢 | 快（保留备份） |

---

## 服务器要求

- **操作系统**: Ubuntu 20.04+ / Debian 11+
- **内存**: 2GB+
- **Go**: 1.21+
- **Node.js**: 18+
- **PM2**: 最新版

---

## 首次部署

### 方式一：使用初始化脚本（推荐）

```bash
# 1. 下载初始化脚本
curl -O https://raw.githubusercontent.com/your-org/ft-api/main/setup-server.sh

# 2. 添加执行权限
chmod +x setup-server.sh

# 3. 运行初始化脚本
sudo ./setup-server.sh
```

脚本会自动完成：
- 安装 Go、Node.js、PM2
- 克隆代码仓库
- 编译项目
- 启动服务

### 方式二：手动部署

```bash
# 1. 安装 Go
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 2. 安装 Node.js 和 PM2
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs
sudo npm install -g pm2

# 3. 克隆代码
sudo mkdir -p /opt/ft-api
sudo git clone git@github.com:your-org/ft-api.git /opt/ft-api
cd /opt/ft-api

# 4. 配置环境变量
cp .env.example .env
vim .env  # 编辑配置

# 5. 编译
go mod download
go build -o new-api .

# 6. 启动服务
pm2 start ecosystem.config.js
pm2 save
pm2 startup
```

---

## 日常更新流程

### 本地开发

```bash
# 1. 修改代码
# 2. 提交到 GitHub
git add .
git commit -m "fix: 修复视频状态同步问题"
git push origin main
```

### 服务器更新

```bash
# 方式一：使用部署脚本（推荐）
cd /opt/ft-api
./deploy.sh deploy

# 方式二：手动更新
cd /opt/ft-api
git pull origin main
go build -o new-api .
pm2 restart ft-api
```

---

## 常用命令

### 部署脚本命令

```bash
./deploy.sh deploy    # 完整部署（拉取 + 编译 + 重启）
./deploy.sh build     # 仅编译
./deploy.sh restart   # 仅重启
./deploy.sh rollback  # 回滚到上一版本
./deploy.sh status    # 查看状态
./deploy.sh logs      # 查看日志
```

### PM2 命令

```bash
pm2 status            # 查看所有服务状态
pm2 logs ft-api       # 查看日志
pm2 logs ft-api --lines 100  # 查看最近 100 行日志
pm2 monit             # 实时监控面板
pm2 restart ft-api    # 重启服务
pm2 stop ft-api       # 停止服务
pm2 delete ft-api     # 删除服务
pm2 save              # 保存当前进程列表
```

---

## 配置文件说明

### ecosystem.config.js

PM2 配置文件，定义了应用的运行参数：

```javascript
module.exports = {
  apps: [{
    name: 'ft-api',
    script: './new-api',
    cwd: '/opt/ft-api',
    instances: 1,
    autorestart: true,
    max_memory_restart: '2G',
    env: {
      GIN_MODE: 'release'
    }
  }]
};
```

### .env

环境变量配置文件：

```bash
# 服务器地址（重要！用于生成视频预览 URL）
SERVER_ADDRESS=https://api.ftai.cc

# 数据库
SQL_DSN=postgres://user:pass@localhost:5432/ftapi

# Redis
REDIS_CONN_STRING=redis://localhost:6379

# Session 密钥
SESSION_SECRET=your-secret-key

# 运行模式
GIN_MODE=release
PORT=3000
```

---

## 目录结构

```
/opt/ft-api/
├── new-api              # 编译后的可执行文件
├── ecosystem.config.js  # PM2 配置
├── deploy.sh            # 部署脚本
├── .env                 # 环境变量
├── logs/                # 日志目录
│   ├── out.log          # 标准输出
│   └── error.log        # 错误日志
├── backups/             # 备份目录（保留最近 5 个版本）
└── data/                # 数据目录
```

---

## 故障排查

### 服务无法启动

```bash
# 1. 查看详细日志
pm2 logs ft-api --lines 200

# 2. 检查端口占用
lsof -i :3000

# 3. 检查环境变量
cat /opt/ft-api/.env
```

### 编译失败

```bash
# 1. 检查 Go 版本
go version

# 2. 清理并重新下载依赖
cd /opt/ft-api
go clean -modcache
go mod download
go build -o new-api .
```

### 回滚到上一版本

```bash
cd /opt/ft-api
./deploy.sh rollback
```

---

## 从 Docker 迁移

### 1. 停止 Docker 容器

```bash
docker stop ft-api
docker rm ft-api
```

### 2. 导出数据（如果需要）

```bash
# 导出数据库（如果使用 Docker 内的数据库）
docker exec postgres pg_dump -U user ftapi > backup.sql
```

### 3. 按照首次部署步骤部署

### 4. 验证服务正常

```bash
curl http://localhost:3000/api/status
```

### 5. 清理 Docker 资源（可选）

```bash
docker system prune -a
```

---

## 安全建议

1. **SSH 密钥**: 使用 SSH 密钥而非密码连接 GitHub
2. **防火墙**: 只开放必要端口（3000、22）
3. **环境变量**: 不要将 `.env` 文件提交到 Git
4. **定期备份**: 定期备份数据库和配置文件

---

**文档版本**: 1.0.0
**最后更新**: 2026-01-28
