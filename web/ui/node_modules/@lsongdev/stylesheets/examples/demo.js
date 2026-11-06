/* Shared behaviors for the CSS Components example pages.
   Handles: explicit theme switching, dropdowns, and vertical submenus. */
(function () {
  'use strict';

  var root = document.documentElement;

  function currentTheme() {
    var fromUrl = new URLSearchParams(location.search).get('theme');
    return fromUrl || localStorage.getItem('demo-theme') || 'light';
  }

  function apply(name, propagate) {
    root.setAttribute('data-theme', name);
    try {
      localStorage.setItem('demo-theme', name);
    } catch (e) { /* private mode */ }
    document.querySelectorAll('[data-set-theme]').forEach(function (btn) {
      btn.classList.toggle('active', btn.getAttribute('data-set-theme') === name);
    });
    if (propagate) {
      document.querySelectorAll('iframe[name="demo-frame"]').forEach(function (frame) {
        var url = new URL(frame.src, location.href);
        url.searchParams.set('theme', name);
        frame.src = url.toString();
      });
    }
  }

  var urlTheme = new URLSearchParams(location.search).get('theme');
  apply(urlTheme || currentTheme(), false);

  document.addEventListener('click', function (event) {
    var btn = event.target.closest('[data-set-theme]');
    if (btn) {
      apply(btn.getAttribute('data-set-theme'), true);
    }
  });

  /* Vertical submenu toggle (menu.css) */  document.addEventListener('click', function (event) {
    var link = event.target.closest('.menu-vertical li > a[data-submenu]');
    if (!link) return;
    event.preventDefault();
    var li = link.parentElement;
    var submenu = li.querySelector(':scope > ul');
    if (!submenu) return;
    var open = submenu.style.display === 'block';
    submenu.style.display = open ? 'none' : 'block';
    li.classList.toggle('menu-open', !open);
  });

  /* Dropdowns (dropdown.css) */
  document.addEventListener('click', function (event) {
    var trigger = event.target.closest('[data-dropdown]');
    if (trigger) {
      event.preventDefault();
      var menu = document.getElementById(trigger.getAttribute('data-dropdown'));
      if (menu) menu.classList.toggle('show');
      return;
    }
    if (!event.target.closest('.dropdown')) {
      document.querySelectorAll('.dropdown-content.show').forEach(function (m) {
        m.classList.remove('show');
      });
    }
  });

  /* Menu dropdowns / context menus (menu.css) */
  document.addEventListener('click', function (event) {
    var trigger = event.target.closest('[data-menu]');
    if (trigger) {
      event.stopPropagation();
      var menu = document.getElementById(trigger.getAttribute('data-menu'));
      if (menu) menu.classList.toggle('menu-open');
      return;
    }
    if (!event.target.closest('.menu-dropdown, .menu-context')) {
      document.querySelectorAll('.menu-dropdown.menu-open, .menu-context.menu-open').forEach(function (m) {
        m.classList.remove('menu-open');
      });
    }
  });

  /* Report page name to the parent hub so it can highlight the active menu item. */
  if (window.parent !== window && location.pathname) {
    window.addEventListener('DOMContentLoaded', function () {
      window.parent.postMessage({ demoPage: location.pathname.split('/').pop() }, '*');
    });
  }
})();
