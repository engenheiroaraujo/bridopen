window.BridopenPage.register("note-card-menu", function (ctx) {
  var grid = document.getElementById("note-grid");
  if (!grid) return;

  function closeAllMenus() {
    grid.querySelectorAll("[data-note-menu-panel]").forEach(function (panel) {
      panel.classList.add("hidden");
      var wrap = panel.closest(".note-card-menu");
      if (!wrap) return;
      var trigger = wrap.querySelector("[data-note-menu-trigger]");
      if (trigger) trigger.setAttribute("aria-expanded", "false");
    });
  }

  function toggleMenu(trigger, panel) {
    var willOpen = panel.classList.contains("hidden");
    closeAllMenus();
    if (willOpen) {
      panel.classList.remove("hidden");
      trigger.setAttribute("aria-expanded", "true");
    }
  }

  grid.addEventListener("click", function (e) {
    var trigger = e.target.closest("[data-note-menu-trigger]");
    if (trigger) {
      e.preventDefault();
      e.stopPropagation();
      var wrap = trigger.closest(".note-card-menu");
      var panel = wrap && wrap.querySelector("[data-note-menu-panel]");
      if (panel) toggleMenu(trigger, panel);
      return;
    }

    if (e.target.closest("[data-note-menu-panel]")) {
      return;
    }

    closeAllMenus();
  });

  // Fechar ao clicar fora / Esc exige listener em document: por ctx.on, para
  // não acumular um par a cada navegação.
  ctx.on(document, "click", function (e) {
    if (!grid.contains(e.target)) {
      closeAllMenus();
    }
  });

  ctx.on(document, "keydown", function (e) {
    if (e.key !== "Escape") return;
    closeAllMenus();
  });
});
