window.BridopenPage.register("account-delete-modal", function (ctx) {
  var modal = document.getElementById("account-delete-modal");
  var form = document.getElementById("account-delete-form");
  var cancelBtn = document.getElementById("account-delete-cancel");
  var confirmBtn = document.getElementById("account-delete-confirm");

  if (!modal || !form) return;

  function openModal() {
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    modal.setAttribute("aria-hidden", "false");
    document.body.classList.add("overflow-hidden");
    if (cancelBtn) cancelBtn.focus();
  }

  function closeModal() {
    modal.classList.add("hidden");
    modal.classList.remove("flex");
    modal.setAttribute("aria-hidden", "true");
    document.body.classList.remove("overflow-hidden");
    if (confirmBtn && window.BridopenActionLock) window.BridopenActionLock.release(confirmBtn);
  }

  ctx.on(document, "click", function (e) {
    if (e.target.closest("[data-account-delete-open]")) {
      e.preventDefault();
      openModal();
    }
  });

  modal.addEventListener("click", function (e) {
    if (e.target.closest("[data-account-delete-dismiss]")) {
      closeModal();
    }
  });

  ctx.on(document, "keydown", function (e) {
    if (e.key !== "Escape") return;
    if (modal.classList.contains("hidden")) return;
    closeModal();
  });

  if (confirmBtn) {
    confirmBtn.addEventListener("click", function () {
      if (form.requestSubmit) {
        form.requestSubmit();
        return;
      }
      if (!form.checkValidity()) {
        form.reportValidity();
        return;
      }
      var releaseAction =
        window.BridopenActionLock && window.BridopenActionLock.begin
          ? window.BridopenActionLock.begin(confirmBtn)
          : null;
      if (!releaseAction) return;

      try {
        form.submit();
      } catch (err) {
        releaseAction();
        throw err;
      }
    });
  }
});
