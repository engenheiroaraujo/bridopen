(function () {
  var storageKey = "bridopen:pending-toast";
  var defaultDuration = 4200;

  function normalizeType(type) {
    if (type === "success" || type === "error" || type === "warning") return type;
    return "info";
  }

  function ensureRoot() {
    var root = document.querySelector("[data-toast-root]");
    if (root) return root;
    if (!document.body) return null;

    root = document.createElement("div");
    root.className = "qn-toast-root";
    root.setAttribute("data-toast-root", "");
    root.setAttribute("aria-live", "polite");
    root.setAttribute("aria-atomic", "false");
    document.body.appendChild(root);
    return root;
  }

  function dismiss(toast) {
    if (!toast || toast.dataset.toastClosing === "true") return;
    toast.dataset.toastClosing = "true";
    if (toast._dismissTimer) {
      window.clearTimeout(toast._dismissTimer);
      toast._dismissTimer = null;
    }
    toast.classList.remove("is-visible");
    window.setTimeout(function () {
      if (toast.parentNode) toast.parentNode.removeChild(toast);
    }, 180);
  }

  function scheduleDismiss(toast, duration) {
    if (!toast || duration <= 0) return;
    if (toast._dismissTimer) window.clearTimeout(toast._dismissTimer);
    toast._dismissTimer = window.setTimeout(function () {
      dismiss(toast);
    }, duration);
  }

  function findDuplicate(root, message, type) {
    var existing = root.querySelectorAll(".qn-toast");
    for (var i = 0; i < existing.length; i += 1) {
      if (existing[i].dataset.toastClosing === "true") continue;
      if (existing[i].dataset.toastMessage === message && existing[i].dataset.toastType === type) {
        return existing[i];
      }
    }
    return null;
  }

  function show(message, type, options) {
    message = String(message || "").trim();
    if (!message) return null;

    var root = ensureRoot();
    if (!root) return null;

    var toastType = normalizeType(type);
    var duration = options && Number(options.duration) > 0 ? Number(options.duration) : defaultDuration;
    var duplicate = findDuplicate(root, message, toastType);
    if (duplicate) {
      duplicate.classList.remove("is-visible");
      window.requestAnimationFrame(function () {
        duplicate.classList.add("is-visible");
      });
      scheduleDismiss(duplicate, duration);
      return {
        close: function () {
          dismiss(duplicate);
        },
      };
    }

    var toast = document.createElement("div");
    var text = document.createElement("p");
    var marker = document.createElement("span");
    var close = document.createElement("button");

    toast.className = "qn-toast qn-toast--" + toastType;
    toast.setAttribute("role", toastType === "error" ? "alert" : "status");
    toast.dataset.toastMessage = message;
    toast.dataset.toastType = toastType;

    marker.className = "qn-toast__marker";
    marker.setAttribute("aria-hidden", "true");

    text.className = "qn-toast__text";
    text.textContent = message;

    close.className = "qn-toast__close";
    close.type = "button";
    close.setAttribute("aria-label", "Fechar mensagem");
    close.textContent = "x";
    close.addEventListener("click", function () {
      dismiss(toast);
    });

    toast.appendChild(marker);
    toast.appendChild(text);
    toast.appendChild(close);
    root.appendChild(toast);

    window.requestAnimationFrame(function () {
      toast.classList.add("is-visible");
    });

    scheduleDismiss(toast, duration);

    return {
      close: function () {
        dismiss(toast);
      },
    };
  }

  function remember(message, type, options) {
    message = String(message || "").trim();
    if (!message) return;
    try {
      window.sessionStorage.setItem(
        storageKey,
        JSON.stringify({
          message: message,
          type: normalizeType(type),
          duration: options && Number(options.duration) > 0 ? Number(options.duration) : defaultDuration,
          createdAt: Date.now(),
        })
      );
    } catch (err) {
      // O toast ainda pode aparecer na pagina atual; persistencia e apenas conveniencia.
    }
  }

  function showPending() {
    var raw = "";
    try {
      raw = window.sessionStorage.getItem(storageKey) || "";
      window.sessionStorage.removeItem(storageKey);
    } catch (err) {
      raw = "";
    }
    if (!raw) return;

    try {
      var data = JSON.parse(raw);
      if (!data || Date.now() - Number(data.createdAt || 0) > 30000) return;
      show(data.message, data.type, { duration: data.duration });
    } catch (err) {
      return;
    }
  }

  function showDeclarativeToasts() {
    document.querySelectorAll("[data-toast-message]").forEach(function (node) {
      var message = node.getAttribute("data-toast-message") || "";
      var type = node.getAttribute("data-toast-type") || "info";
      var duration = Number(node.getAttribute("data-toast-duration") || "0");
      show(message, type, duration > 0 ? { duration: duration } : null);
      node.removeAttribute("data-toast-message");
    });
  }

  function ready(fn) {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", fn, { once: true });
      return;
    }
    fn();
  }

  window.BridopenToast = {
    show: show,
    success: function (message, options) {
      return show(message, "success", options);
    },
    error: function (message, options) {
      return show(message, "error", options);
    },
    warning: function (message, options) {
      return show(message, "warning", options);
    },
    info: function (message, options) {
      return show(message, "info", options);
    },
    remember: remember,
  };
  window.showToast = show;

  // A API window.showToast pertence ao shell e é definida uma única vez. Só a
  // leitura dos toasts declarados no conteúdo precisa rodar a cada página.
  window.BridopenPage.register("toast-page", function () {
    showPending();
    showDeclarativeToasts();
  });
})();
