window.BridopenPage.register("note-tags-page", function (ctx) {
  function normalizeHexColor(raw) {
    if (!raw) return null;
    var s = String(raw).trim();
    if (!s.startsWith("#")) s = "#" + s;
    var body = s.slice(1).toLowerCase();
    if (/^[0-9a-f]{3}$/.test(body)) {
      return "#" + body[0] + body[0] + body[1] + body[1] + body[2] + body[2];
    }
    if (/^[0-9a-f]{6}$/.test(body)) return "#" + body;
    return null;
  }

  function closeTagMenus(exceptTrigger) {
    document.querySelectorAll("[data-tag-menu-trigger]").forEach(function (trigger) {
      if (trigger === exceptTrigger) return;
      var menu = document.getElementById(trigger.getAttribute("aria-controls"));
      if (!menu) return;
      menu.classList.add("hidden");
      menu.setAttribute("aria-hidden", "true");
      trigger.setAttribute("aria-expanded", "false");
    });
  }

  function initTagMenus() {
    document.querySelectorAll("[data-tag-menu-trigger]").forEach(function (trigger) {
      var menu = document.getElementById(trigger.getAttribute("aria-controls"));
      if (!menu) return;

      trigger.addEventListener("click", function (event) {
        event.preventDefault();
        event.stopPropagation();
        var open = !menu.classList.contains("hidden");
        closeTagMenus(trigger);
        menu.classList.toggle("hidden", open);
        menu.setAttribute("aria-hidden", open ? "true" : "false");
        trigger.setAttribute("aria-expanded", open ? "false" : "true");
      });

      menu.addEventListener("click", function (event) {
        if (event.target.closest("[data-tag-edit-open]")) closeTagMenus();
      });
    });

    ctx.on(document, "click", function (event) {
      if (event.target.closest(".qn-me-card-menu")) return;
      closeTagMenus();
    });
  }

  function initTagFilters() {
    var nameInput = document.querySelector("[data-tag-name-filter]");
    var colorInput = document.querySelector("[data-tag-color-filter]");
    var clearButton = document.querySelector("[data-tag-filter-clear]");
    var empty = document.querySelector("[data-tag-filter-empty]");
    var items = Array.prototype.slice.call(document.querySelectorAll("[data-tag-item]"));
    if (!nameInput || !colorInput || !items.length) return;

    function applyFilters() {
      var name = nameInput.value.trim().toLowerCase();
      var color = colorInput.value.trim().toLowerCase();
      if (color && !color.startsWith("#")) color = "#" + color;

      var visible = 0;
      items.forEach(function (item) {
        var itemName = (item.getAttribute("data-tag-name") || "").toLowerCase();
        var itemColor = (item.getAttribute("data-tag-color") || "").toLowerCase();
        var show = (!name || itemName.indexOf(name) !== -1) && (!color || itemColor.indexOf(color) !== -1);
        item.classList.toggle("hidden", !show);
        if (show) visible += 1;
      });
      if (empty) empty.classList.toggle("hidden", visible !== 0);
    }

    ["input", "change", "keyup"].forEach(function (eventName) {
      nameInput.addEventListener(eventName, applyFilters);
      colorInput.addEventListener(eventName, applyFilters);
    });

    if (clearButton) {
      clearButton.addEventListener("click", function () {
        nameInput.value = "";
        colorInput.value = "";
        applyFilters();
        nameInput.focus();
      });
    }
  }

  function bindColorHex(form, colorId, hexId) {
    var colorInput = document.getElementById(colorId);
    var hexInput = document.getElementById(hexId);
    if (!form || !colorInput || !hexInput) return;

    function setColor(value) {
      var normalized = normalizeHexColor(value) || "#2f4538";
      colorInput.value = normalized;
      hexInput.value = normalized;
      hexInput.setCustomValidity("");
    }

    colorInput.addEventListener("input", function () {
      hexInput.value = colorInput.value.toLowerCase();
      hexInput.setCustomValidity("");
    });

    colorInput.addEventListener("change", function () {
      hexInput.value = colorInput.value.toLowerCase();
      hexInput.setCustomValidity("");
    });

    hexInput.addEventListener("input", function () {
      hexInput.setCustomValidity("");
      var normalized = normalizeHexColor(hexInput.value);
      if (normalized) colorInput.value = normalized;
    });

    hexInput.addEventListener("blur", function () {
      if (hexInput.value.trim() === "") {
        setColor(colorInput.value);
        return;
      }
      var normalized = normalizeHexColor(hexInput.value);
      if (normalized) setColor(normalized);
      else hexInput.setCustomValidity("Use #rgb ou #rrggbb (apenas 0-9 e a-f).");
    });

    form.addEventListener("submit", function (event) {
      var normalized = normalizeHexColor(hexInput.value);
      if (!normalized) {
        event.preventDefault();
        hexInput.setCustomValidity("Digite uma cor válida: #rgb ou #rrggbb.");
        hexInput.reportValidity();
        return;
      }
      setColor(normalized);
    });

    setColor(hexInput.value || colorInput.value);
    form.__setTagColor = setColor;
  }

  function initModal(config) {
    var modal = document.getElementById(config.modalId);
    var form = document.getElementById(config.formId);
    var nameInput = document.getElementById(config.nameId);
    if (!modal || !form || !nameInput) return null;

    var lastFocus = null;

    function openModal(button) {
      lastFocus = document.activeElement;
      if (config.edit && button) {
        form.action = "/tags/" + button.getAttribute("data-tag-id");
        nameInput.value = button.getAttribute("data-tag-name") || "";
        if (form.__setTagColor) form.__setTagColor(button.getAttribute("data-tag-color"));
      } else if (!config.edit) {
        form.action = "/tags";
        nameInput.value = "";
        if (form.__setTagColor) form.__setTagColor("#2f4538");
      }
      modal.classList.remove("hidden");
      modal.classList.add("flex");
      modal.setAttribute("aria-hidden", "false");
      closeTagMenus();
      window.setTimeout(function () {
        nameInput.focus();
        nameInput.select();
      }, 0);
    }

    function closeModal() {
      modal.classList.add("hidden");
      modal.classList.remove("flex");
      modal.setAttribute("aria-hidden", "true");
      if (lastFocus && typeof lastFocus.focus === "function") lastFocus.focus();
    }

    document.querySelectorAll(config.openSelector).forEach(function (button) {
      button.addEventListener("click", function () {
        openModal(button);
      });
    });

    document.querySelectorAll(config.closeSelector).forEach(function (button) {
      button.addEventListener("click", closeModal);
    });

    if (!modal.classList.contains("hidden")) {
      modal.setAttribute("aria-hidden", "false");
      window.setTimeout(function () {
        nameInput.focus();
        nameInput.select();
      }, 0);
    }
    return closeModal;
  }

  function initTagModals() {
    var createForm = document.getElementById("tag-create-form");
    var editForm = document.getElementById("tag-edit-form");
    bindColorHex(createForm, "tag-create-color", "tag-create-color-hex");
    bindColorHex(editForm, "tag-edit-color", "tag-edit-color-hex");

    var closeCreate = initModal({
      modalId: "tag-create-modal",
      formId: "tag-create-form",
      nameId: "tag-create-name",
      openSelector: "[data-tag-create-open]",
      closeSelector: "[data-tag-create-close]",
      edit: false,
    });
    var closeEdit = initModal({
      modalId: "tag-edit-modal",
      formId: "tag-edit-form",
      nameId: "tag-edit-name",
      openSelector: "[data-tag-edit-open]",
      closeSelector: "[data-tag-edit-close]",
      edit: true,
    });

    ctx.on(document, "keydown", function (event) {
      if (event.key !== "Escape") return;
      closeTagMenus();
      if (closeCreate) closeCreate();
      if (closeEdit) closeEdit();
    });
  }

  initTagMenus();
  initTagFilters();
  initTagModals();
});
