import dayjs from 'dayjs';

// 计数展示：<1000 原样；<1w 用 k；≥1w 用 w（对齐 relation 原型 3.6k / 1.2w）
export function formatCount(n: number): string {
  if (!n || n < 0) {
    return '0';
  }
  if (n < 1000) {
    return String(n);
  }
  if (n < 10000) {
    return (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k';
  }
  return (n / 10000).toFixed(1).replace(/\.0$/, '') + 'w';
}

// 轻量相对时间（不引入 dayjs relativeTime 插件；> 7 天回退绝对日期）
export function relativeTime(ms: number): string {
  const diff = Date.now() - ms;
  const minute = 60_000;
  const hour = 3_600_000;
  const day = 86_400_000;
  if (diff < minute) {
    return '刚刚';
  }
  if (diff < hour) {
    return `${Math.floor(diff / minute)} 分钟前`;
  }
  if (diff < day) {
    return `${Math.floor(diff / hour)} 小时前`;
  }
  if (diff < 7 * day) {
    return `${Math.floor(diff / day)} 天前`;
  }
  return dayjs(ms).format('YYYY-MM-DD');
}

// 加入时长（他人主页头部「加入 2 年」）
export function joinedFor(ms: number): string {
  if (!ms) {
    return '';
  }
  const days = Math.floor((Date.now() - ms) / 86_400_000);
  if (days < 1) {
    return '今天加入';
  }
  if (days < 30) {
    return `加入 ${days} 天`;
  }
  if (days < 365) {
    return `加入 ${Math.floor(days / 30)} 个月`;
  }
  return `加入 ${Math.floor(days / 365)} 年`;
}
