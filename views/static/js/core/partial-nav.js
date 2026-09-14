/**
 * Navegação parcial: troca apenas o conteúdo principal, mantendo o sidebar,
 * o rodapé e os scripts no DOM.
 *
 * Sem JavaScript, links e formulários mantêm a navegação nativa. Erros de
 * requisição preservam o conteúdo atual e são informados por toast.
 *
 * Diferenças deliberadas em relação à referência (gestoronline), que navega por
 * `link.pathname` e ignora destino de mesmo caminho:
 *   - navegamos pelo href completo, preservando query string e hash;
 *   - permitimos mesmo caminho com query diferente, porque os filtros da Home
 *     (?q=, ?color=, ?tag=, ?sort=, ?pinned=) são exatamente esse caso.
 */
(function () {
  var HEADER = "X-Nav-Mode";
  var VALUE = "partial";

  var main = document.getElementById("main-content");
  var modalsSlot = document.getElementById("page-modals");
  if (!main) return;

  var inFlight = null;

  function fullLoad(url) {
    window.location.href = url;
  }

  /** Só intercepta o que é seguro trocar sem recarregar o shell. */
  function isPartialCandidate(link) {
    if (!link || !link.href) return false;
    if (link.hasAttribute("download")) return false;
    if (link.hasAttribute("data-nav-full")) return false;
    if (link.target && link.target.toLowerCase() !== "_self") return false;
    if (link.origin !== window.location.origin) return false;
    if (link.getAttribute("href").charAt(0) === "#") return false;
    return true;
  }

  function setBusy(busy) {
    if (busy) {
      main.setAttribute("aria-busy", "true");
    } else {
      main.removeAttribute("aria-busy");
    }
  }

  function navigate(url, push, options) {
    options = options || {};
    var isMutation = options.method === "POST";
    var previousURL = window.location.href;
    var scrollX = window.scrollX;
    var scrollY = window.scrollY;
    var destination = new URL(url, window.location.origin).href;
    if (inFlight) inFlight.abort();
    var controller = new AbortController();
    inFlight = controller;
    setBusy(true);

    return fetch(url, {
      method: options.method || "GET",
      body: options.body,
      headers: (function () {
        var h = {};
        h[HEADER] = VALUE;
        return h;
      })(),
      credentials: "same-origin",
      redirect: "follow",
      // A later navigation may discard the result, but must not cancel a write.
      signal: isMutation ? undefined : controller.signal,
    })
      .then(function (res) {
        if (controller.signal.aborted) return null;
        var type = res.headers.get("Content-Type") || "";
        if (!res.ok) throw new Error("Request failed");
        if (type.indexOf("application/json") === -1) {
          // Authentication and other full-page destinations still need their shell.
          if (!isMutation || res.redirected) {
            fullLoad(res.url || url);
            return null;
          }
          throw new Error("Expected a page fragment");
        }
        if (res.redirected) destination = res.url;
        return res.json();
      })
      .then(function (fragment) {
        if (!fragment || controller.signal.aborted) return;
        if (typeof fragment.html !== "string" || typeof fragment.title !== "string") {
          throw new Error("Invalid page fragment");
        }

        window.BridopenPage.unmountAll();

        main.innerHTML = fragment.html;
        if (modalsSlot) modalsSlot.innerHTML = fragment.modals || "";
        document.title = fragment.title;

        if (push && destination !== window.location.href) {
          window.history.pushState({ partial: true }, "", destination);
        }

        var target = new URL(destination, window.location.origin);
        window.BridopenSidebar.syncActive(target.pathname);

        // Posiciona a rolagem ANTES de montar os módulos: uma página pode
        // querer rolar até uma seção específica (o realce de /me), e o
        // scrollTo(0,0) desta etapa desfaria esse posicionamento.
        if (destination === previousURL) {
          window.scrollTo({ left: scrollX, top: scrollY, behavior: "instant" });
        } else if (target.hash) {
          var anchor = document.getElementById(target.hash.slice(1));
          if (anchor && typeof anchor.scrollIntoView === "function") {
            anchor.scrollIntoView();
          }
        } else {
          window.scrollTo({ left: 0, top: 0, behavior: "instant" });
        }
        // preventScroll: o foco em [tabindex="-1"] faria o browser rolar até o
        // main, desfazendo o posicionamento que acabamos de aplicar.
        main.focus({ preventScroll: true });

        window.BridopenPage.mountAll();
        return true;
      })
      .catch(function () {
        if (controller.signal.aborted) return;
        if (window.BridopenToast) {
          window.BridopenToast.error(isMutation
            ? "Não foi possível confirmar a atualização. Confira a nota antes de tentar novamente."
            : "Não foi possível atualizar a página. Tente novamente.");
        }
        return false;
      })
      .then(function (updated) {
        if (inFlight === controller) {
          inFlight = null;
          setBusy(false);
        }
        return updated;
      });
  }

  document.addEventListener("click", function (event) {
    if (event.defaultPrevented || event.button !== 0) return;
    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    var target = event.target;
    if (!target || typeof target.closest !== "function") return;
    var link = target.closest("a[href]");
    if (!isPartialCandidate(link)) return;

    event.preventDefault();
    navigate(link.href, true);
  });

  /**
   * Formulários GET (a busca e os filtros da Home) também navegam parcial: o
   * destino é uma URL com query string, exatamente o caso que a referência não
   * cobre. As ações de estado das notas também usam fragmentos após o redirect.
   * Os demais formulários POST mantêm seu tratamento existente.
   */
  document.addEventListener("submit", function (event) {
    if (event.defaultPrevented) return;
    var form = event.target;
    if (!form) return;
    if (form.hasAttribute("data-nav-full")) return;
    if (form.target && form.target.toLowerCase() !== "_self") return;

    var action = new URL(form.action || window.location.href, window.location.origin);
    if (action.origin !== window.location.origin) return;

    var method = form.method.toLowerCase();
    var isNoteAction = method === "post" && /^\/note\/\d+\/(pin|unpin|archive|unarchive|restore)$/.test(action.pathname);
    if (method !== "get" && !isNoteAction) return;

    if (isNoteAction) {
      event.preventDefault();
      if (form.dataset.qnPartialSubmitting === "true") return;
      form.dataset.qnPartialSubmitting = "true";
      navigate(action.href, true, {
        method: "POST",
        body: new URLSearchParams(new FormData(form)),
      }).finally(function () {
        delete form.dataset.qnPartialSubmitting;
        if (window.BridopenActionLock) window.BridopenActionLock.releaseForm(form);
      });
      return;
    }

    var params = new URLSearchParams(new FormData(form));
    var query = params.toString();
    event.preventDefault();
    navigate(action.pathname + (query ? "?" + query : ""), true);
  });

  window.addEventListener("popstate", function () {
    navigate(window.location.href, false);
  });

  window.BridopenNavigation = {
    visit: function (url) { return navigate(url, true); },
  };
})();
