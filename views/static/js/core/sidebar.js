/**
 * Sidebar global: drawer no mobile, grupos recolhíveis e destaque do item ativo.
 *
 * Vive no shell e nunca é remontado — por isso os listeners aqui são
 * registrados uma única vez, fora do ciclo de vida das páginas. A navegação
 * parcial só chama `syncActive()` para reposicionar o destaque.
 */
window.BridopenSidebar = (function () {
  var GROUP_STATE_KEY = "qn-sidebar-groups";

  var sidebar = document.getElementById("app-sidebar");
  var backdrop = document.getElementById("app-sidebar-backdrop");
  var openButton = document.querySelector("[data-sidebar-open]");
  var lastFocused = null;

  function readGroupState() {
    try {
      return JSON.parse(sessionStorage.getItem(GROUP_STATE_KEY) || "{}") || {};
    } catch (e) {
      return {};
    }
  }

  function writeGroupState(state) {
    try {
      sessionStorage.setItem(GROUP_STATE_KEY, JSON.stringify(state));
    } catch (e) {
      /* modo privado ou storage cheio: o grupo só perde a memória entre cargas */
    }
  }

  function setGroup(button, open, persist) {
    var panel = document.getElementById(button.getAttribute("aria-controls"));
    if (!panel) return;
    button.setAttribute("aria-expanded", open ? "true" : "false");
    button.classList.toggle("qn-sidebar-link--open", open);
    panel.classList.toggle("is-open", open);
    panel.hidden = !open;

    if (!persist) return;
    var name = button.getAttribute("data-sidebar-group");
    if (!name) return;
    var state = readGroupState();
    state[name] = open;
    writeGroupState(state);
  }

  /** Restaura, na carga da página, os grupos que o utilizador deixou abertos. */
  function restoreGroups() {
    if (!sidebar) return;
    var state = readGroupState();
    sidebar.querySelectorAll("[data-sidebar-group]").forEach(function (button) {
      var name = button.getAttribute("data-sidebar-group");
      if (!Object.prototype.hasOwnProperty.call(state, name)) return;
      // O grupo do item ativo fica aberto de qualquer forma; a memória só
      // consegue abrir grupos, nunca fechar o que contém a página atual.
      if (!state[name] && button.classList.contains("qn-sidebar-link--open")) return;
      setGroup(button, !!state[name], false);
    });
  }

  function isOpen() {
    return document.body.classList.contains("sidebar-open");
  }

  function openSidebar() {
    if (!sidebar || isOpen()) return;
    lastFocused = document.activeElement;
    document.body.classList.add("sidebar-open");
    if (openButton) openButton.setAttribute("aria-expanded", "true");
    var focusable = sidebar.querySelector("[data-sidebar-close]") || sidebar;
    focusable.focus();
  }

  function closeSidebar() {
    if (!sidebar || !isOpen()) return;
    document.body.classList.remove("sidebar-open");
    if (openButton) openButton.setAttribute("aria-expanded", "false");
    if (lastFocused && typeof lastFocused.focus === "function") lastFocused.focus();
    lastFocused = null;
  }

  /**
   * Reposiciona o destaque do item ativo depois de uma navegação parcial.
   * Compara apenas o caminho: os filtros da Home vivem na query string e não
   * devem tirar o destaque de "Home".
   */
  function syncActive(pathname) {
    if (!sidebar) return;

    sidebar.querySelectorAll(".qn-sidebar-link--active, .qn-sidebar-sublink--active").forEach(function (el) {
      el.classList.remove("qn-sidebar-link--active");
      el.classList.remove("qn-sidebar-sublink--active");
      el.removeAttribute("aria-current");
    });

    var links = sidebar.querySelectorAll(".qn-sidebar-link[href], .qn-sidebar-sublink[href]");
    var match = null;
    for (var i = 0; i < links.length; i += 1) {
      if (links[i].pathname === pathname) {
        match = links[i];
        break;
      }
    }
    if (!match) return;

    var isSub = match.classList.contains("qn-sidebar-sublink");
    match.classList.add(isSub ? "qn-sidebar-sublink--active" : "qn-sidebar-link--active");
    match.setAttribute("aria-current", "page");

    // Abre o grupo que contém o item ativo, sem gravar como preferência.
    var panel = match.closest(".qn-sidebar-subnav");
    if (!panel) return;
    var button = sidebar.querySelector('[aria-controls="' + panel.id + '"]');
    if (button) setGroup(button, true, false);
  }

  if (sidebar) {
    restoreGroups();

    sidebar.addEventListener("click", function (event) {
      var toggle = event.target.closest("[data-sidebar-group]");
      if (toggle && sidebar.contains(toggle)) {
        setGroup(toggle, toggle.getAttribute("aria-expanded") !== "true", true);
        return;
      }
      // Um link fecha o drawer no mobile; no desktop a classe não está ativa.
      if (event.target.closest("a[href]")) closeSidebar();
    });

    document.addEventListener("click", function (event) {
      if (event.target.closest("[data-sidebar-open]")) {
        openSidebar();
        return;
      }
      if (event.target.closest("[data-sidebar-close]")) closeSidebar();
    });

    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && isOpen()) closeSidebar();
    });

    window.addEventListener("resize", function () {
      if (window.innerWidth >= 1024) closeSidebar();
    });
  }

  return {
    syncActive: syncActive,
    close: closeSidebar,
  };
})();
