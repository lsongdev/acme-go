# Vendored browser dependencies

Served as-is so the module works offline and needs no build step.

| File       | Package      | Version | Source |
|------------|--------------|---------|--------|
| preact.mjs | preact       | 11.0.2  | https://cdn.jsdelivr.net/npm/preact@11.0.2/dist/preact.mjs |
| hooks.mjs  | preact/hooks | 11.0.2  | https://cdn.jsdelivr.net/npm/preact@11.0.2/hooks/dist/hooks.mjs |
| htm.mjs    | htm          | 3.1.1   | https://cdn.jsdelivr.net/npm/htm@3.1.1/dist/htm.module.js |
| stylesheets.css | @lsongdev/stylesheets | 1.1.0 | https://cdn.jsdelivr.net/npm/@lsongdev/stylesheets@1.1.0/ |
| scripts/ | @lsongdev/scripts | 0.1.0-rc.2 | https://cdn.jsdelivr.net/npm/@lsongdev/scripts@0.1.0-rc.2/ |

Local modification: `hooks.mjs` imports `./preact.mjs` instead of the bare
specifier `preact`, so no import map is needed.

`stylesheets.css` concatenates base.css (without its palette import),
card.css, input.css, button.css, table.css, badge.css, alert.css, dialog.css,
page.css, field.css, tabs.css, key-value.css, spinner.css, log.css, toast.css,
theme/light.css and theme/dark.css, with their `@import` lines removed; each
part is marked with a comment.

`scripts/` holds dom/dialog.js, dom/stylesheets.js and the react/components
files the pages use, at their paths in the package. Local modification:
`scripts/react/react.js` re-exports `../../preact.mjs` and `../../hooks.mjs`
instead of loading Preact from unpkg, so the components share the pages'
runtime.
