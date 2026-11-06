/* Sidebar controller for the demo hub (index.html).
   Replaces the old Navigation class, which targeted a navigation.css that
   does not exist in this library. */
(function () {
  'use strict';

  var sidebar = document.getElementById('app-sidebar');
  var toggle = document.getElementById('sidebar-toggle');

  if (toggle && sidebar) {
    toggle.addEventListener('click', function () {
      sidebar.classList.toggle('sidebar-open');
    });
  }

  /* Highlight the menu item whose target matches the current iframe page.
     Works across hub <-> iframe navigations via postMessage. */
  var frame = document.querySelector('iframe[name="demo-frame"]');

  function setActive(page) {
    var links = document.querySelectorAll('#app-menu a[target="demo-frame"]');
    links.forEach(function (link) {
      var active = link.getAttribute('href') === page;
      link.classList.toggle('menu-item-active', active);
      var li = link.parentElement;
      if (li) li.classList.toggle('menu-item-active', active);
    });
  }

  if (frame) {
    setActive(new URL(frame.src, location.href).pathname.split('/').pop());

    frame.addEventListener('load', function () {
      try {
        setActive(new URL(frame.contentWindow.location.href).pathname.split('/').pop());
      } catch (e) { /* cross-origin guard */ }
    });

    window.addEventListener('message', function (event) {
      if (event.data && event.data.demoPage) {
        setActive(event.data.demoPage);
      }
    });
  }
})();
