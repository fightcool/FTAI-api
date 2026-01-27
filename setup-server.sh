#!/bin/bash
#
# FT-API 服务器初始化脚本
# 在新服务器上首次部署时运行
#

set -e

# 配置
APP_DIR="/opt/ft-api"
GO_VERSION="1.21.6"
NODE_VERSION="18"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否为 root 用户
check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "请使用 root 用户运行此脚本"
        exit 1
    fi
}

# 安装系统依赖
install_dependencies() {
    log_info "安装系统依赖..."

    # 更新包管理器
    apt-get update -y

    # 安装基础工具
    apt-get install -y \
        curl \
        wget \
        git \
        build-essential \
        ca-certificates \
        gnupg \
        lsb-release
}

# 安装 Go
install_go() {
    if command -v go &> /dev/null; then
        log_info "Go 已安装: $(go version)"
        return
    fi

    log_info "安装 Go $GO_VERSION..."

    cd /tmp
    wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
    rm "go${GO_VERSION}.linux-amd64.tar.gz"

    # 添加到 PATH
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    echo 'export GOPATH=$HOME/go' >> /etc/profile
    echo 'export PATH=$PATH:$GOPATH/bin' >> /etc/profile

    source /etc/profile

    log_info "Go 安装完成: $(go version)"
}

# 安装 Node.js 和 PM2
install_node_pm2() {
    if command -v node &> /dev/null; then
        log_info "Node.js 已安装: $(node -v)"
    else
        log_info "安装 Node.js $NODE_VERSION..."

        curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash -
        apt-get install -y nodejs

        log_info "Node.js 安装完成: $(node -v)"
    fi

    if command -v pm2 &> /dev/null; then
        log_info "PM2 已安装: $(pm2 -v)"
    else
        log_info "安装 PM2..."
        npm install -g pm2
        log_info "PM2 安装完成: $(pm2 -v)"
    fi

    # 设置 PM2 开机自启
    pm2 startup systemd -u root --hp /root
}

# 克隆代码仓库
clone_repo() {
    if [ -d "$APP_DIR/.git" ]; then
        log_info "代码仓库已存在，跳过克隆"
        return
    fi

    log_info "克隆代码仓库..."

    # 提示输入仓库地址
    read -p "请输入 Git 仓库地址 (例如: git@github.com:your-org/ft-api.git): " REPO_URL

    if [ -z "$REPO_URL" ]; then
        log_error "仓库地址不能为空"
        exit 1
    fi

    mkdir -p "$APP_DIR"
    git clone "$REPO_URL" "$APP_DIR"

    log_info "代码克隆完成"
}

# 配置环境变量
setup_env() {
    log_info "配置环境变量..."

    ENV_FILE="$APP_DIR/.env"

    if [ -f "$ENV_FILE" ]; then
        log_warn ".env 文件已存在，跳过配置"
        return
    fi

    cat > "$ENV_FILE" << 'EOF'
# FT-API 环境配置
# 请根据实际情况修改以下配置

# 服务器地址（用于生成视频预览 URL 等）
SERVER_ADDRESS=https://api.ftai.cc

# 数据库配置
SQL_DSN=postgres://user:password@localhost:5432/ftapi?sslmode=disable

# Redis 配置
REDIS_CONN_STRING=redis://localhost:6379

# Session 密钥（请修改为随机字符串）
SESSION_SECRET=your-random-secret-key-here

# 运行模式
GIN_MODE=release

# 端口
PORT=3000
EOF

    log_warn "请编辑 $ENV_FILE 文件，配置正确的数据库和 Redis 连接信息"
}

# 创建目录结构
setup_dirs() {
    log_info "创建目录结构..."

    mkdir -p "$APP_DIR/logs"
    mkdir -p "$APP_DIR/backups"
    mkdir -p "$APP_DIR/data"

    # 设置权限
    chmod +x "$APP_DIR/deploy.sh" 2>/dev/null || true
}

# 首次编译
first_build() {
    log_info "首次编译项目..."

    cd "$APP_DIR"

    # 确保 Go 环境变量生效
    export PATH=$PATH:/usr/local/go/bin

    # 下载依赖
    go mod download

    # 编译
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o new-api .

    if [ -f "new-api" ]; then
        chmod +x new-api
        log_info "编译成功"
    else
        log_error "编译失败"
        exit 1
    fi
}

# 启动服务
start_service() {
    log_info "启动服务..."

    cd "$APP_DIR"

    # 加载环境变量
    if [ -f ".env" ]; then
        export $(cat .env | grep -v '^#' | xargs)
    fi

    # 使用 PM2 启动
    pm2 start ecosystem.config.js
    pm2 save

    log_info "服务已启动"
    pm2 status
}

# 显示完成信息
show_complete() {
    echo ""
    echo "=========================================="
    echo -e "${GREEN}FT-API 初始化完成！${NC}"
    echo "=========================================="
    echo ""
    echo "后续操作："
    echo "1. 编辑配置文件: vim $APP_DIR/.env"
    echo "2. 重启服务: cd $APP_DIR && ./deploy.sh restart"
    echo ""
    echo "常用命令："
    echo "  ./deploy.sh deploy   - 从 GitHub 拉取并部署"
    echo "  ./deploy.sh restart  - 重启服务"
    echo "  ./deploy.sh logs     - 查看日志"
    echo "  ./deploy.sh status   - 查看状态"
    echo "  ./deploy.sh rollback - 回滚到上一版本"
    echo ""
    echo "PM2 命令："
    echo "  pm2 status           - 查看所有服务状态"
    echo "  pm2 logs ft-api      - 查看日志"
    echo "  pm2 monit            - 监控面板"
    echo ""
}

# 主函数
main() {
    log_info "========== FT-API 服务器初始化 =========="

    check_root
    install_dependencies
    install_go
    install_node_pm2
    clone_repo
    setup_dirs
    setup_env
    first_build
    start_service
    show_complete
}

main "$@"
