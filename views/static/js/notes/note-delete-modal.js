(function () {
  var modal = document.getElementById("note-delete-modal");
  var titleEl = document.getElementById("note-delete-modal-title");
  var descEl = document.getElementById("note-delete-modal-desc");
  var cancelBtn = document.getElementById("note-delete-cancel");
  var confirmBtn = document.getElementById("note-delete-confirm");
  var csrfTokenMeta = document.querySelector('meta[name="csrf-token"]');
  var deleteUrl = "";
  var redirectTo = "";
  var reloadOnSuccess = false;
  var errorMessage = "Não foi possível concluir a ação. Tente novamente.";
  var successMessage = "Ação concluída.";

  if (!modal) return;

  function getCSRFToken() {
    if (csrfTokenMeta) {
      var metaToken = csrfTokenMeta.getAttribute("content") || "";
      if (metaToken) return metaToken;
    }
    if (confirmBtn) {
      return confirmBtn.getAttribute("data-csrf-token") || "";
    }
    return "";
  }

  function closeNoteMenus() {
    document.querySelectorAll("[data-note-menu-panel]").forEach(function (panel) {
      panel.classList.add("hidden");
      var wrap = panel.closest(".note-card-menu");
      if (!wrap) return;
      var trigger = wrap.querySelector("[data-note-menu-trigger]");
      if (trigger) trigger.setAttribute("aria-expanded", "false");
    });
  }

  function showFeedback(message, type) {
    if (window.BridopenToast && window.BridopenToast.show) {
      window.BridopenToast.show(message, type);
      return;
    }
    if (window.showToast) {
      window.showToast(message, type);
      return;
    }
    console[type === "error" ? "error" : "log"](message);
  }

  function rememberFeedback(message, type) {
    if (window.BridopenToast && window.BridopenToast.remember) {
      window.BridopenToast.remember(message, type);
    }
  }

  function readJSON(res) {
    var contentType = res.headers.get("content-type") || "";
    if (contentType.indexOf("application/json") === -1) {
      return Promise.resolve(null);
    }
    return res.json().catch(function () {
      return null;
    });
  }

  function openModal(btn) {
    deleteUrl = btn.getAttribute("data-note-delete-url") || "";
    if (!deleteUrl) return;
    redirectTo = btn.getAttribute("data-note-delete-redirect") || "";
    reloadOnSuccess = btn.getAttribute("data-note-delete-reload") === "true";
    errorMessage =
      btn.getAttribute("data-note-delete-error") ||
      "Não foi possível concluir a ação. Tente novamente.";
    successMessage = btn.getAttribute("data-note-delete-success") || "Ação concluída.";

    if (titleEl) {
      titleEl.textContent = btn.getAttribute("data-note-delete-title") || "Excluir anota\u00e7\u00e3o?";
    }
    if (descEl) {
      descEl.textContent =
        btn.getAttribute("data-note-delete-message") ||
        "Esta a\u00e7\u00e3o n\u00e3o pode ser desfeita.";
    }
    if (confirmBtn) {
      confirmBtn.textContent = btn.getAttribute("data-note-delete-confirm") || "Excluir";
    }
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    modal.setAttribute("aria-hidden", "false");
    document.body.classList.add("overflow-hidden");
    if (confirmBtn && window.BridopenActionLock) window.BridopenActionLock.release(confirmBtn);
    if (cancelBtn) cancelBtn.focus();
  }

  function closeModal() {
    modal.classList.add("hidden");
    modal.classList.remove("flex");
    modal.setAttribute("aria-hidden", "true");
    document.body.classList.remove("overflow-hidden");
    deleteUrl = "";
    redirectTo = "";
    reloadOnSuccess = false;
    errorMessage = "Não foi possível concluir a ação. Tente novamente.";
    successMessage = "Ação concluída.";
    if (confirmBtn && window.BridopenActionLock) window.BridopenActionLock.release(confirmBtn);
  }

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-note-delete-action]");
    if (!btn) return;
    e.preventDefault();
    closeNoteMenus();
    openModal(btn);
  });

  modal.addEventListener("click", function (e) {
    if (e.target.closest("[data-note-delete-dismiss]")) {
      closeModal();
    }
  });

  document.addEventListener("keydown", function (e) {
    if (e.key !== "Escape") return;
    if (modal.classList.contains("hidden")) return;
    closeModal();
  });

  if (!confirmBtn) return;

  confirmBtn.addEventListener("click", function () {
    var csrfToken = getCSRFToken();
    if (!csrfToken) {
      console.warn("CSRF token vazio no HTML renderizado.");
    }
    var releaseAction =
      window.BridopenActionLock && window.BridopenActionLock.begin
        ? window.BridopenActionLock.begin(confirmBtn)
        : null;
    if (!releaseAction) return;
    fetch(deleteUrl, {
      method: "DELETE",
      headers: {
        "X-CSRF-Token": csrfToken,
        "X-Requested-With": "XMLHttpRequest",
        Accept: "application/json"
      },
      credentials: "same-origin",
    })
      .then(function (res) {
        return readJSON(res).then(function (data) {
          if (!res.ok && res.status !== 302 && res.status !== 303) {
            var actionError = new Error("note action failed");
            actionError.userMessage = (data && data.message) || errorMessage;
            throw actionError;
          }

          var message = (data && data.message) || successMessage;
          var nextUrl = redirectTo || (data && data.redirect) || "/";
          if (reloadOnSuccess) nextUrl = window.location.href;
          closeModal();
          if (window.BridopenNavigation) {
            return window.BridopenNavigation.visit(nextUrl).then(function (updated) {
              if (updated) showFeedback(message, "success");
            });
          }
          rememberFeedback(message, "success");
          window.location.assign(nextUrl);
        });
      })
      .catch(function (err) {
        releaseAction();
        showFeedback((err && err.userMessage) || errorMessage, "error");
      });
  });
})();
