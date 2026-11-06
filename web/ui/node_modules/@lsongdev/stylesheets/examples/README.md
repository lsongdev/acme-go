# CSS Components — 示例页

本目录演示样式库的真实 CSS API。打开 `index.html` 进入演示中心：左侧为 `sidebar.css` 纵向菜单，右侧 iframe 展示各组件，顶栏可切换主题（亮色 / 暗色 / 谷歌 / 粉红）。

## 运行方式

直接用浏览器打开 `index.html` 即可（无需构建）。所有页面均为静态 HTML + CSS，除演示交互外不依赖运行时 JS。

## 页面索引

| 分类 | 文件 | 演示的组件 / 模块 |
| --- | --- | --- |
| — | `index.html` | 演示中心（sidebar + menu + 主题切换） |
| — | `overview.html` | 总览、stat-grid、section、alert、progressbar、table |
| 基础 | `tokens.html` | 语义令牌、局部主题、组件令牌覆盖 |
| 基础 | `base.html` | base.css 语义令牌与排版 |
| 基础 | `color.html` | color.css 调色板与文字工具类 |
| 基础 | `font.html` | font.css / text.css 字族字号工具类 |
| 基础 | `prose.html` | prose.css / `.prose` 文章与 Markdown 排版 |
| 基础 | `animation.html` | animation.css 呼吸动画关键帧 |
| 布局 | `flex.html` | flex.css 弹性布局工具类 |
| 布局 | `grid.html` | grid.css 12 列栅格与响应式断点 |
| 表单 | `button.html` | button.css 变体、尺寸、分组、图标按钮 |
| 表单 | `input.html` | input.css 输入框、状态、前后缀组合 |
| 表单 | `form.html` | form.css 字段分组、帮助/错误文本、内联表单 |
| 表单 | `checkbox.html` | switch.css 开关组件 |
| 导航 | `navigation.html` | navbar.css 导航栏 |
| 导航 | `menu.html` | menu.css 横向 / 纵向菜单、下拉与右键菜单 |
| 导航 | `dropdown.html` | dropdown.css 下拉菜单 |
| 展示 | `card.html` | card.css 卡片 |
| 展示 | `list.html` | list.css 列表行 |
| 展示 | `link.html` | link.css 链接 |
| 展示 | `badge.html` | badge.css 状态徽章 |
| 展示 | `icon.html` | icon.css Font Awesome 与 SVG 掩码图标 |
| 展示 | `dialog.html` | dialog.css 原生 dialog 与抽屉 / 底部面板 / toast |

## 主题机制

文章排版的字体策略、宿主集成和视觉检查场景见 [prose.md](./prose.md)。

- 主题通过 `data-theme="<名称>"` 显式开启，写在 `<html>` 或任意子树均可。
- 演示页同时引入 `theme/light.css`、`theme/dark.css`、`theme/google.css`、`theme/pink.css`，由 `demo.js` 读取 `?theme=` 参数或 `localStorage` 决定当前主题。
- 注意：`animation.css`、`icon.css`、`sidebar.css`、`section.css`、`progressbar.css`、`link.css` 未包含在 `index.css` 中，需要时请单独引入。

## 演示基础设施

- `demo.css` — 演示页共用的头部、区块、色板、代码块样式。
- `demo.js` — 主题切换、下拉 / 子菜单展开、页面向演示中心上报当前页。
- `navigation.js` — 演示中心的侧边栏控制（折叠、高亮当前页）。
