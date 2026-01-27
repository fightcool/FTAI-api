/**
 * PM2 部署配置文件
 * 用于 FT-API 的生产环境部署
 */

module.exports = {
  apps: [
    {
      name: 'ft-api',
      // Go 编译后的可执行文件路径
      script: './new-api',
      // 工作目录
      cwd: '/opt/ft-api',
      // 环境变量
      env: {
        NODE_ENV: 'production',
        GIN_MODE: 'release',
        // 数据库配置（从环境变量或 .env 文件读取）
        SQL_DSN: process.env.SQL_DSN || '',
        REDIS_CONN_STRING: process.env.REDIS_CONN_STRING || '',
        SESSION_SECRET: process.env.SESSION_SECRET || '',
      },
      // 实例数量（Go 程序通常单实例即可，内部已有 goroutine 并发）
      instances: 1,
      // 执行模式
      exec_mode: 'fork',
      // 自动重启
      autorestart: true,
      // 监听文件变化（生产环境关闭）
      watch: false,
      // 最大内存限制（超过后自动重启）
      max_memory_restart: '2G',
      // 日志配置
      log_date_format: 'YYYY-MM-DD HH:mm:ss',
      error_file: '/opt/ft-api/logs/error.log',
      out_file: '/opt/ft-api/logs/out.log',
      merge_logs: true,
      // 重启延迟
      restart_delay: 3000,
      // 最大重启次数（10次失败后停止）
      max_restarts: 10,
      // 优雅关闭超时
      kill_timeout: 5000,
      // 等待应用就绪的时间
      wait_ready: true,
      listen_timeout: 10000,
    }
  ],

  // 部署配置
  deploy: {
    production: {
      // SSH 用户
      user: 'root',
      // 服务器地址（多台服务器用数组）
      host: ['your-server-ip'],
      // 部署的分支
      ref: 'origin/main',
      // Git 仓库地址
      repo: 'git@github.com:your-org/ft-api.git',
      // 服务器上的部署路径
      path: '/opt/ft-api',
      // SSH 选项
      ssh_options: 'StrictHostKeyChecking=no',
      // 部署前执行的命令（在本地）
      'pre-deploy-local': '',
      // 部署后执行的命令（在服务器）
      'post-deploy': 'cd /opt/ft-api/current && ./deploy.sh',
      // 环境变量
      env: {
        NODE_ENV: 'production',
        GIN_MODE: 'release'
      }
    }
  }
};
