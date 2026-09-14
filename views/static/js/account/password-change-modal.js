window.BridopenPage.register("password-change-modal", function (ctx) {
  var modal = document.getElementById("password-change-modal");
  if (!modal) return;

  var form = document.getElementById("passwordChangeForm");
  var firstInput = document.getElementById("account_current_secret");
  var closeBtn = modal.querySelector("[data-password-change-dismiss]");
  var hasServerFeedback = modal.querySelector('[role="alert"], [role="status"]');

  function isOpen() {
    return !modal.classList.contains("hidden");
  }

  function openModal() {
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
    if (firstInput) {
      firstInput.focus();
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
    if (e.target.closest("[data-password-change-open]")) {
      e.preventDefault();
      openModal();
      return;
    }
    if (e.target.closest("[data-password-change-dismiss]")) {
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
      if (firstInput) {
        firstInput.focus();
        return;
      }
      if (closeBtn) closeBtn.focus();
    }, 0);
  }
});
