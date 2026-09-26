window.BridopenPage.register("recovery-email-modal", function (ctx) {
  var modal = document.getElementById("recovery-email-modal");
  if (!modal) return;

  var form = document.getElementById("recoveryEmailForm");
  var emailInput = document.getElementById("recovery_email");
  var closeBtn = modal.querySelector("[data-recovery-email-dismiss]");
  var hasServerFeedback = modal.querySelector('[role="alert"], [role="status"]');

  function isOpen() {
    return !modal.classList.contains("hidden");
  }

  function closeAccountMenus() {
    document.querySelectorAll("[data-me-menu-trigger]").forEach(function (trigger) {
      var menu = document.getElementById(trigger.getAttribute("aria-controls"));
      if (!menu) return;
      menu.classList.add("hidden");
      menu.setAttribute("aria-hidden", "true");
      trigger.setAttribute("aria-expanded", "false");
    });
  }

  function openModal() {
    closeAccountMenus();
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    modal.setAttribute("aria-hidden", "false");
    document.body.classList.add("overflow-hidden");
    if (form && !hasServerFeedback) {
      form.querySelectorAll('input[type="password"]').forEach(function (input) {
        input.value = "";
        input.type = "password";
      });
    }
    if (emailInput) {
      emailInput.focus();
      return;
    }
    if (closeBtn) closeBtn.focus();
  }

  function closeModal() {
    modal.classList.add("hidden");
    modal.classList.remove("flex");
    modal.setAttribute("aria-hidden", "true");
    document.body.classList.remove("overflow-hidden");
  }

  ctx.on(document, "click", function (e) {
    if (e.target.closest("[data-recovery-email-open]")) {
      e.preventDefault();
      openModal();
      return;
    }
    if (e.target.closest("[data-recovery-email-dismiss]")) {
      closeModal();
    }
  });

  ctx.on(document, "keydown", function (e) {
    if (e.key !== "Escape") return;
    if (!isOpen()) return;
    closeModal();
  });

  if (isOpen()) {
    document.body.classList.add("overflow-hidden");
    window.setTimeout(function () {
      if (emailInput) {
        emailInput.focus();
        return;
      }
      if (closeBtn) closeBtn.focus();
    }, 0);
  }
});
