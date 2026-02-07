#!/bin/bash
#
# FT-API 部署脚本
# 用于从 GitHub 拉取代码并重新编译部署
#

set -e

# 配置
APP_NAME="ft-api"
APP_DIR="/opt/ft-api"
BINARY_NAME="new-api"
LOG_DIR="$APP_DIR/logs"
BACKUP_DIR="$APP_DIR/backups"
GO_VERSION="1.21"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') $1"
}

# 检查 Go 环境
check_go() {
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装，请先安装 Go $GO_VERSION+"
        exit 1
    fi

    GO_INSTALLED=$(go version | grep -oP 'go\d+\.\d+' | head -1)
    log_info "Go 版本: $GO_INSTALLED"
}

# 检查 PM2
check_pm2() {
    if ! command -v pm2 &> /dev/null; then
        log_error "PM2 未安装，请先安装: npm install -g pm2"
        exit 1
    fi
    log_info "PM2 已安装"
}

# 创建必要目录
setup_dirs() {
    mkdir -p "$LOG_DIR"
    mkdir -p "$BACKUP_DIR"
    log_info "目录已创建"
}

# 备份当前版本
backup_current() {
    if [ -f "$APP_DIR/$BINARY_NAME" ]; then
        BACKUP_FILE="$BACKUP_DIR/${BINARY_NAME}_$(date '+%Y%m%d_%H%M%S')"
        cp "$APP_DIR/$BINARY_NAME" "$BACKUP_FILE"
        log_info "已备份当前版本到: $BACKUP_FILE"

        # 保留最近 5 个备份
        cd "$BACKUP_DIR"
        ls -t ${BINARY_NAME}_* 2>/dev/null | tail -n +6 | xargs -r rm -f
        log_info "已清理旧备份，保留最近 5 个"
    fi
}

# 拉取最新代码
pull_code() {
    log_info "拉取最新代码..."
    cd "$APP_DIR"

    # 保存本地修改（如果有）
    git stash --include-untracked 2>/dev/null || true

    # 拉取最新代码
    git fetch origin
    git reset --hard origin/main

    log_info "代码已更新到最新版本"
    git log -1 --oneline
}

# 编译项目（含前端构建）
build_project() {
    log_info "开始编译项目..."
    cd "$APP_DIR"

    # 构建前端（React + Vite）
    if [ -d "web" ] && [ -f "web/package.json" ]; then
        log_info "构建前端..."
        cd web
        npm install --production=false 2>/dev/null || npm install
        npm run build
        cd "$APP_DIR"
        log_info "前端构建完成"
    fi

    # 下载依赖
    go mod download

    # 编译
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BINARY_NAME" .

    if [ -f "$BINARY_NAME" ]; then
        chmod +x "$BINARY_NAME"
        # 复制为 PM2 使用的应用名，保持一致
        cp "$BINARY_NAME" "$APP_NAME"
        chmod +x "$APP_NAME"
        log_info "编译成功: $BINARY_NAME -> $APP_NAME"
        ls -lh "$APP_NAME"
    else
        log_error "编译失败"
        exit 1
    fi
}

# 重启服务
restart_service() {
    log_info "重启服务..."

    # 检查 PM2 中是否存在该应用
    if pm2 list | grep -q "$APP_NAME"; then
        pm2 restart "$APP_NAME"
        log_info "服务已重启"
    else
        # 首次启动：直接用二进制文件路径注册
        pm2 start "$APP_DIR/$APP_NAME" --name "$APP_NAME"
        pm2 save
        log_info "服务已启动"
    fi

    # 等待服务启动（数据库迁移需要约60秒）
    sleep 5

    # 检查服务状态
    pm2 status "$APP_NAME"
}

# 健康检查
health_check() {
    log_info "执行健康检查...（服务启动约需60秒，请耐心等待）"

    # 等待服务完全启动（数据库迁移需要约60秒）
    sleep 65

    # 检查 API 是否响应
    HEALTH_URL="http://localhost:3000/api/status"

    for i in {1..5}; do
        if curl -s -o /dev/null -w "%{http_code}" "$HEALTH_URL" | grep -q "200"; then
            log_info "健康检查通过"
            return 0
        fi
        log_warn "健康检查失败，重试 $i/5...（等待10秒）"
        sleep 10
    done

    log_error "健康检查失败，请检查日志"
    pm2 logs "$APP_NAME" --lines 50
    return 1
}

# 回滚到上一版本
rollback() {
    log_warn "执行回滚..."

    LATEST_BACKUP=$(ls -t "$BACKUP_DIR"/${BINARY_NAME}_* 2>/dev/null | head -1)

    if [ -z "$LATEST_BACKUP" ]; then
        log_error "没有可用的备份"
        exit 1
    fi

    cp "$LATEST_BACKUP" "$APP_DIR/$BINARY_NAME"
    chmod +x "$APP_DIR/$BINARY_NAME"
    pm2 restart "$APP_NAME"

    log_info "已回滚到: $LATEST_BACKUP"
}

# 显示帮助
show_help() {
    echo "FT-API 部署脚本"
    echo ""
    echo "用法: $0 [命令]"
    echo ""
    echo "命令:"
    echo "  deploy    完整部署流程（拉取代码 + 编译 + 重启）"
    echo "  build     仅编译项目"
    echo "  restart   仅重启服务"
    echo "  rollback  回滚到上一版本"
    echo "  status    查看服务状态"
    echo "  logs      查看实时日志"
    echo "  help      显示帮助"
}

# 主函数
main() {
    case "${1:-deploy}" in
        deploy)
            log_info "========== 开始部署 =========="
            check_go
            check_pm2
            setup_dirs
            backup_current
            pull_code
            build_project
            restart_service
            health_check
            log_info "========== 部署完成 =========="
            ;;
        build)
            check_go
            build_project
            ;;
        restart)
            restart_service
            ;;
        rollback)
            rollback
            ;;
        status)
            pm2 status "$APP_NAME"
            ;;
        logs)
            pm2 logs "$APP_NAME" --lines 100
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $1"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
