# FT-API PM2 部署指南

> **版本**: 2.0.0
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
- **Go**: 1.24+（重要：项目依赖需要 Go 1.24 或更高版本）
- **Node.js**: 18+
- **PM2**: 最新版

---

## 首次部署

### 步骤 1：安装 Go 1.24+

```bash
cd /tmp
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
go version  # 应显示 go version go1.24.x linux/amd64
```

### 步骤 2：安装 Node.js 和 PM2

```bash
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs
sudo npm install -g pm2
```

### 步骤 3：克隆代码

```bash
sudo mkdir -p /opt/ft-api
cd /opt
sudo git clone https://github.com/fightcool/FTAI-api.git ft-api
cd /opt/ft-api
```

### 步骤 4：配置环境变量

```bash
# 复制生产环境配置模板
cp .env.production .env

# 编辑配置（根据实际情况修改）
vim .env
```

**.env 文件内容示例：**

```bash
# 数据库连接（阿里云 RDS PostgreSQL）
SQL_DSN=postgres://用户名:密码@数据库地址:5432/数据库名?sslmode=disable

# Redis 连接
REDIS_CONN_STRING=redis://172.17.0.1:6379

# 会话密钥
SESSION_SECRET=your_secret_key

# 服务器地址（用于生成视频预览 URL）
SERVER_ADDRESS=https://api.ftai.cc

# 缓存配置
MEMORY_CACHE_ENABLED=true
SYNC_FREQUENCY=60

# 日志配置
ERROR_LOG_ENABLED=true

# 运行模式
GIN_MODE=release
PORT=3000

# 时区
TZ=Asia/Shanghai
```

### 步骤 5：创建前端占位目录

```bash
# Go embed 需要 web/dist 目录存在
mkdir -p web/dist
echo '<!DOCTYPE html><html><head><title>FT-API</title></head><body>API Server</body></html>' > web/dist/index.html
```

### 步骤 6：编译项目

```bash
export PATH=/usr/local/go/bin:$PATH
go mod download
go build -o new-api .
```

### 步骤 7：启动服务

```bash
pm2 start ecosystem.config.js
pm2 save
pm2 startup  # 设置开机自启
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
cd /opt/ft-api
git pull origin main
export PATH=/usr/local/go/bin:$PATH
go build -o new-api .
pm2 restart ft-api
```

或使用部署脚本：

```bash
cd /opt/ft-api
./deploy.sh deploy
```

---

## 常用命令

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

### 部署脚本命令

```bash
./deploy.sh deploy    # 完整部署（拉取 + 编译 + 重启）
./deploy.sh build     # 仅编译
./deploy.sh restart   # 仅重启
./deploy.sh rollback  # 回滚到上一版本
./deploy.sh status    # 查看状态
./deploy.sh logs      # 查看日志
```

---

## 配置文件说明

### ecosystem.config.js

PM2 配置文件，**自动从 .env 文件读取环境变量**：

```javascript
const fs = require('fs');
const path = require('path');

// 读取 .env 文件并解析为环境变量对象
function loadEnvFile(envPath) {
  const env = {};
  try {
    const envFile = fs.readFileSync(envPath, 'utf8');
    envFile.split('\n').forEach(line => {
      line = line.trim();
      if (!line || line.startsWith('#')) return;
      const [key, ...valueParts] = line.split('=');
      if (key && valueParts.length > 0) {
        env[key.trim()] = valueParts.join('=').trim();
      }
    });
  } catch (err) {
    console.warn(`Warning: Could not read ${envPath}:`, err.message);
  }
  return env;
}

const envFile = loadEnvFile(path.join(__dirname, '.env'));

module.exports = {
  apps: [{
    name: 'ft-api',
    script: './new-api',
    cwd: '/opt/ft-api',
    env: {
      ...envFile,
      NODE_ENV: 'production',
      GIN_MODE: envFile.GIN_MODE || 'release',
    },
    instances: 1,
    exec_mode: 'fork',
    autorestart: true,
    max_memory_restart: '2G',
    log_date_format: 'YYYY-MM-DD HH:mm:ss',
    error_file: '/opt/ft-api/logs/error.log',
    out_file: '/opt/ft-api/logs/out.log',
  }]
};
```

### .env

环境变量配置文件（**不要提交到 Git**）：

```bash
# 数据库连接
SQL_DSN=postgres://用户名:密码@数据库地址:5432/数据库名?sslmode=disable

# Redis 连接
REDIS_CONN_STRING=redis://172.17.0.1:6379

# 会话密钥
SESSION_SECRET=your_secret_key

# 服务器地址（重要！用于生成视频预览 URL）
SERVER_ADDRESS=https://api.ftai.cc

# 缓存配置
MEMORY_CACHE_ENABLED=true
SYNC_FREQUENCY=60

# 日志配置
ERROR_LOG_ENABLED=true

# 运行模式
GIN_MODE=release
PORT=3000

# 时区
TZ=Asia/Shanghai
```

---

## 目录结构

```
/opt/ft-api/
├── new-api              # 编译后的可执行文件
├── ecosystem.config.js  # PM2 配置（自动读取 .env）
├── deploy.sh            # 部署脚本
├── .env                 # 环境变量（不提交到 Git）
├── .env.production      # 生产环境配置模板
├── web/dist/            # 前端静态文件目录
├── logs/                # 日志目录
│   ├── out.log          # 标准输出
│   └── error.log        # 错误日志
├── backups/             # 备份目录（保留最近 5 个版本）
└── data/                # 数据目录
```

---

## 故障排查

### 问题 1：Go 版本过低

**错误信息：**
```
go: golang.org/x/crypto@v0.45.0 requires go >= 1.24.0 (running go 1.21.6)
```

**解决方案：**
```bash
cd /tmp
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=/usr/local/go/bin:$PATH
go version
```

### 问题 2：web/dist 目录不存在

**错误信息：**
```
main.go:33:12: pattern web/dist: cannot embed directory web/dist: contains no embeddable files
```

**解决方案：**
```bash
mkdir -p web/dist
echo '<!DOCTYPE html><html><body>API</body></html>' > web/dist/index.html
```

### 问题 3：环境变量未加载

**错误信息：**
```
SQL_DSN not set, using SQLite as database
REDIS_CONN_STRING not set, Redis is not enabled
```

**解决方案：**
1. 确保 `.env` 文件存在且配置正确
2. 确保 `ecosystem.config.js` 包含 `.env` 文件加载逻辑
3. 重新启动服务：
```bash
pm2 delete ft-api
pm2 start ecosystem.config.js
```

### 问题 4：数据库连接失败

**检查步骤：**
```bash
# 1. 查看日志
pm2 logs ft-api --lines 50

# 2. 检查 .env 配置
cat /opt/ft-api/.env

# 3. 测试数据库连接
psql "postgres://用户名:密码@数据库地址:5432/数据库名?sslmode=disable"
```

### 问题 5：服务无法启动

```bash
# 1. 查看详细日志
pm2 logs ft-api --lines 200

# 2. 检查端口占用
lsof -i :3000

# 3. 检查可执行文件
ls -la /opt/ft-api/new-api
```

---

## 从 Docker 迁移

### 1. 获取 Docker 环境变量

```bash
# 查看 Docker 容器的环境变量
docker inspect ft-api | grep -A 50 "Env"

# 或查看 docker-compose.yml
cat docker-compose.prod.yml
```

### 2. 停止 Docker 容器

```bash
docker stop ft-api
docker rm ft-api
```

### 3. 配置 PM2 环境

将 Docker 中的环境变量复制到 `/opt/ft-api/.env` 文件。

### 4. 启动 PM2 服务

```bash
cd /opt/ft-api
pm2 start ecosystem.config.js
pm2 save
```

### 5. 验证服务正常

```bash
curl http://localhost:3000/api/status
pm2 logs ft-api --lines 30
```

日志应显示：
- 连接到 PostgreSQL（而非 SQLite）
- Redis 已启用
- 任务进度轮询正常

### 6. 清理 Docker 资源（可选）

```bash
# 保留数据库和 Redis 容器（如果需要）
docker stop ftai-api-container
docker rm ftai-api-container

# 或完全清理
docker system prune -a
```

---

## 安全建议

1. **SSH 密钥**: 使用 SSH 密钥而非密码连接 GitHub
2. **防火墙**: 只开放必要端口（3000、22）
3. **环境变量**: 不要将 `.env` 文件提交到 Git（已在 .gitignore 中）
4. **定期备份**: 定期备份数据库和配置文件
5. **日志监控**: 定期检查错误日志

---

## 生产环境信息

**当前部署配置：**

| 配置项 | 值 |
|--------|-----|
| 服务器 | 阿里云 ECS |
| 数据库 | 阿里云 RDS PostgreSQL |
| Redis | 宿主机本地 Redis (172.17.0.1:6379) |
| API 地址 | https://api.ftai.cc |
| 端口 | 3000 |
| Go 版本 | 1.24.12 |
| PM2 进程名 | ft-api |

---

**文档版本**: 2.0.0
**最后更新**: 2026-01-28
