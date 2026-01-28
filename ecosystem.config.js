/**
 * PM2 部署配置文件
 * 用于 FT-API 的生产环境部署
 *
 * 环境变量从 .env 文件读取
 */

const fs = require('fs');
const path = require('path');

// 读取 .env 文件并解析为环境变量对象
function loadEnvFile(envPath) {
  const env = {};
  try {
    const envFile = fs.readFileSync(envPath, 'utf8');
    envFile.split('\n').forEach(line => {
      // 跳过空行和注释
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

// 加载 .env 文件
const envFile = loadEnvFile(path.join(__dirname, '.env'));

module.exports = {
  apps: [
    {
      name: 'ft-api',
      // Go 编译后的可执行文件路径
      script: './new-api',
      // 工作目录
      cwd: '/opt/ft-api',
      // 环境变量（从 .env 文件加载）
      env: {
        ...envFile,
        NODE_ENV: 'production',
        GIN_MODE: envFile.GIN_MODE || 'release',
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
  ]
};
