// Strings of the shared components, chosen by the interface language the
// module was mounted with (ctx.lang).
const zh = {
  loading: '加载中…', close: '关闭', remove: '移除', cancel: '取消', confirm: '确定',
  noOutput: '暂无输出', copy: '复制', copied: '已复制', network: '无法连接到服务器',
  password: '密码', signIn: '登录', signOut: '退出登录', wrongPassword: '密码错误', menu: '菜单',
  notFound: '页面不存在', loadFailed: '加载失败',
  signInFailed: status => `登录失败（HTTP ${status}）`,
  panelFailed: status => `加载面板失败（HTTP ${status}）`,
  moduleFailed: (title, message) => `模块 ${title} 加载失败：${message}`,
};

const en = {
  loading: 'Loading…', close: 'Close', remove: 'Remove', cancel: 'Cancel', confirm: 'OK',
  noOutput: 'No output', copy: 'Copy', copied: 'Copied', network: 'Cannot reach the server',
  password: 'Password', signIn: 'Sign in', signOut: 'Sign out', wrongPassword: 'Wrong password', menu: 'Menu',
  notFound: 'Page not found', loadFailed: 'Failed to load',
  signInFailed: status => `Sign-in failed (HTTP ${status})`,
  panelFailed: status => `Failed to load the panel (HTTP ${status})`,
  moduleFailed: (title, message) => `Module ${title} failed to load: ${message}`,
};

let current = 'zh-CN';

export function setLang(lang) { current = lang || 'zh-CN'; }

export const isChinese = () => current.toLowerCase().startsWith('zh');

export function t(key, ...args) {
  const dict = isChinese() ? zh : en;
  const value = dict[key] ?? key;
  return typeof value === 'function' ? value(...args) : value;
}
