window.BridopenPage.register("edit-personal-info", function (ctx) {
  var menuTrigger = document.getElementById("btnOpenPersonalInfoMenu");
  var menu = document.getElementById("personalInfoMenu");
  var openModalBtn = document.getElementById("btnOpenEditPersonalInfoModal");
  var modal = document.getElementById("editPersonalInfoModal");
  var form = document.getElementById("editPersonalInfoForm");
  var cancelBtn = document.getElementById("btnCancelEditPersonalInfo");

  var personalNameAllowedRe;
  try {
    personalNameAllowedRe = /[^\p{Script=Latin}' \-]/gu;
  } catch (e) {
    personalNameAllowedRe = /[^A-Za-z\u00C0-\u024F' \-]/g;
  }

  var personalPhoneAllowedRe = /[^\d\s()+\-]/g;

  var nameControlKeys = {
    Backspace: true,
    Delete: true,
    Tab: true,
    Escape: true,
    Enter: true,
    ArrowLeft: true,
    ArrowRight: true,
    ArrowUp: true,
    ArrowDown: true,
    Home: true,
    End: true,
  };

  function sanitizePersonalName(value) {
    return String(value || "").replace(personalNameAllowedRe, "");
  }

  function isAllowedNameText(text) {
    if (!text) return true;
    return sanitizePersonalName(text) === text;
  }

  function attachPersonalNameInput(input) {
    if (!input) return;

    input.addEventListener("keydown", function (e) {
      if (e.isComposing || e.key === "Dead" || e.key === "Process") return;
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      if (nameControlKeys[e.key]) return;
      if (e.key === " " || e.key === "-" || e.key === "'") return;
      if (e.key.length === 1 && isAllowedNameText(e.key)) return;
      e.preventDefault();
    });

    input.addEventListener("beforeinput", function (e) {
      if (e.isComposing) return;
      if (e.inputType === "insertText" || e.inputType === "insertCompositionText") {
        if (e.data && !isAllowedNameText(e.data)) {
          e.preventDefault();
        }
      }
    });

    input.addEventListener("input", function () {
      var cleaned = sanitizePersonalName(input.value);
      if (cleaned !== input.value) {
        input.value = cleaned;
      }
    });

    input.addEventListener("paste", function (e) {
      var pasted = (e.clipboardData || window.clipboardData).getData("text");
      if (!pasted || isAllowedNameText(pasted)) return;
      e.preventDefault();
      var start = input.selectionStart != null ? input.selectionStart : input.value.length;
      var end = input.selectionEnd != null ? input.selectionEnd : input.value.length;
      var merged = sanitizePersonalName(input.value.slice(0, start) + pasted + input.value.slice(end));
      input.value = merged;
      var pos = Math.min(start + sanitizePersonalName(pasted).length, merged.length);
      if (input.setSelectionRange) input.setSelectionRange(pos, pos);
    });

    input.value = sanitizePersonalName(input.value);
  }

  function sanitizePersonalPhone(value) {
    return String(value || "").replace(personalPhoneAllowedRe, "");
  }

  function isAllowedPhoneText(text) {
    if (!text) return true;
    return sanitizePersonalPhone(text) === text;
  }

  function attachPersonalPhoneInput(input) {
    if (!input) return;

    input.addEventListener("keydown", function (e) {
      if (e.isComposing || e.key === "Dead" || e.key === "Process") return;
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      if (nameControlKeys[e.key]) return;
      if (e.key.length === 1 && isAllowedPhoneText(e.key)) return;
      e.preventDefault();
    });

    input.addEventListener("beforeinput", function (e) {
      if (e.isComposing) return;
      if (e.inputType === "insertText" || e.inputType === "insertCompositionText") {
        if (e.data && !isAllowedPhoneText(e.data)) {
          e.preventDefault();
        }
      }
    });

    input.addEventListener("input", function () {
      var cleaned = sanitizePersonalPhone(input.value);
      if (cleaned !== input.value) {
        input.value = cleaned;
      }
    });

    input.addEventListener("paste", function (e) {
      var pasted = (e.clipboardData || window.clipboardData).getData("text");
      if (!pasted || isAllowedPhoneText(pasted)) return;
      e.preventDefault();
      var start = input.selectionStart != null ? input.selectionStart : input.value.length;
      var end = input.selectionEnd != null ? input.selectionEnd : input.value.length;
      var merged = sanitizePersonalPhone(input.value.slice(0, start) + pasted + input.value.slice(end));
      input.value = merged;
      var pos = Math.min(start + sanitizePersonalPhone(pasted).length, merged.length);
      if (input.setSelectionRange) input.setSelectionRange(pos, pos);
    });

    input.value = sanitizePersonalPhone(input.value);
  }

  if (!menuTrigger || !menu || !modal || !form) return;

  attachPersonalNameInput(document.getElementById("first_name"));
  attachPersonalNameInput(document.getElementById("last_name"));
  attachPersonalPhoneInput(document.getElementById("phone_number"));

  function closeMenu() {
    menu.classList.add("hidden");
    menu.setAttribute("aria-hidden", "true");
    menuTrigger.setAttribute("aria-expanded", "false");
  }

  function openMenu() {
    menu.classList.remove("hidden");
    menu.setAttribute("aria-hidden", "false");
    menuTrigger.setAttribute("aria-expanded", "true");
  }

  function toggleMenu() {
    if (menu.classList.contains("hidden")) {
      openMenu();
    } else {
      closeMenu();
    }
  }

  function openModal() {
    closeMenu();
    modal.classList.remove("hidden");
    modal.classList.add("flex");
    modal.setAttribute("aria-hidden", "false");
    document.body.classList.add("overflow-hidden");
    var firstInput = document.getElementById("first_name");
    if (firstInput) firstInput.focus();
  }

  function closeModal() {
    modal.classList.add("hidden");
    modal.classList.remove("flex");
    modal.setAttribute("aria-hidden", "true");
    document.body.classList.remove("overflow-hidden");
    menuTrigger.focus();
  }

  if (modal.classList.contains("flex")) {
    document.body.classList.add("overflow-hidden");
    var focusTarget = document.getElementById("first_name");
    if (focusTarget) focusTarget.focus();
  }

  menuTrigger.addEventListener("click", function (e) {
    e.preventDefault();
    e.stopPropagation();
    toggleMenu();
  });

  if (openModalBtn) {
    openModalBtn.addEventListener("click", function (e) {
      e.preventDefault();
      openModal();
    });
  }

  modal.addEventListener("click", function (e) {
    if (e.target.closest("[data-edit-personal-dismiss]")) {
      closeModal();
    }
  });

  if (cancelBtn) {
    cancelBtn.addEventListener("click", function () {
      closeModal();
    });
  }

  ctx.on(document, "click", function (e) {
    if (e.target.closest("#btnOpenPersonalInfoMenu") || e.target.closest("#personalInfoMenu")) {
      return;
    }
    closeMenu();
  });

  ctx.on(document, "keydown", function (e) {
    if (e.key !== "Escape") return;
    if (!menu.classList.contains("hidden")) {
      closeMenu();
      return;
    }
    if (!modal.classList.contains("hidden")) {
      closeModal();
    }
  });
});

window.BridopenPage.register("account-action-menus", function (ctx) {
  document.querySelectorAll("[data-me-menu-trigger]").forEach(function (trigger) {
    if (trigger.id === "btnOpenPersonalInfoMenu") return;

    var menu = document.getElementById(trigger.getAttribute("aria-controls"));
    if (!menu) return;

    function closeMenu() {
      menu.classList.add("hidden");
      menu.setAttribute("aria-hidden", "true");
      trigger.setAttribute("aria-expanded", "false");
    }

    function openMenu() {
      document.querySelectorAll("[data-me-menu-trigger]").forEach(function (otherTrigger) {
        if (otherTrigger === trigger) return;
        var otherMenu = document.getElementById(otherTrigger.getAttribute("aria-controls"));
        if (!otherMenu) return;
        otherMenu.classList.add("hidden");
        otherMenu.setAttribute("aria-hidden", "true");
        otherTrigger.setAttribute("aria-expanded", "false");
      });
      menu.classList.remove("hidden");
      menu.setAttribute("aria-hidden", "false");
      trigger.setAttribute("aria-expanded", "true");
    }

    trigger.addEventListener("click", function (e) {
      e.preventDefault();
      e.stopPropagation();
      if (menu.classList.contains("hidden")) {
        openMenu();
      } else {
        closeMenu();
      }
    });

    menu.addEventListener("click", function (e) {
      if (e.target.closest("[data-password-change-open]")) {
        closeMenu();
      }
    });

    ctx.on(document, "click", function (e) {
      if (e.target.closest(".qn-me-card-menu")) return;
      closeMenu();
    });

    ctx.on(document, "keydown", function (e) {
      if (e.key === "Escape") closeMenu();
    });
  });
});
