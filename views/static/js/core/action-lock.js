(function () {
  var lockAttr = "data-qn-action-locked";
  var previousDisabledAttr = "data-qn-action-was-disabled";

  function setLocked(button, locked) {
    if (!button) return;
    if (locked) {
      button.setAttribute(previousDisabledAttr, button.disabled ? "true" : "false");
      button.setAttribute(lockAttr, "true");
      button.disabled = true;
      button.setAttribute("aria-disabled", "true");
      return;
    }

    var wasDisabled = button.getAttribute(previousDisabledAttr) === "true";
    button.removeAttribute(lockAttr);
    button.removeAttribute(previousDisabledAttr);
    button.disabled = wasDisabled;
    button.setAttribute("aria-disabled", wasDisabled ? "true" : "false");
  }

  function begin(button) {
    if (!button || button.disabled || button.getAttribute(lockAttr) === "true") {
      return null;
    }

    setLocked(button, true);
    var released = false;
    return function release() {
      if (released) return;
      released = true;
      setLocked(button, false);
    };
  }

  function submitterFromEvent(event, form) {
    if (event.submitter && event.submitter.matches("button, input")) {
      return event.submitter;
    }
    var active = document.activeElement;
    if (active && form.contains(active) && active.matches('button[type="submit"], input[type="submit"]')) {
      return active;
    }
    return form.querySelector('button[type="submit"], input[type="submit"]');
  }

  function installFormSubmitLock() {
    document.addEventListener(
      "submit",
      function (event) {
        var form = event.target;
        if (!form || !form.matches("form")) return;
        if (form.dataset.qnActionLock === "off") return;

        if (form.dataset.qnSubmitting === "true") {
          event.preventDefault();
          event.stopImmediatePropagation();
          return;
        }

        var button = submitterFromEvent(event, form);
        var release = begin(button);
        if (!release) return;
        form.dataset.qnSubmitting = "true";
        form.qnActionRelease = release;

        window.setTimeout(function () {
          if (!event.defaultPrevented) return;
          if (form.dataset.qnPartialSubmitting === "true") return;
          window.BridopenActionLock.releaseForm(form);
        }, 0);
      },
      true
    );
  }

  window.BridopenActionLock = {
    begin: begin,
    release: function (button) {
      setLocked(button, false);
    },
    releaseForm: function (form) {
      if (!form) return;
      if (typeof form.qnActionRelease === "function") {
        form.qnActionRelease();
      }
      delete form.dataset.qnSubmitting;
      delete form.qnActionRelease;
    },
  };

  installFormSubmitLock();

  window.addEventListener("pageshow", function () {
    document.querySelectorAll("form").forEach(function (form) {
      window.BridopenActionLock.releaseForm(form);
    });
    document.querySelectorAll("button[" + lockAttr + "], input[" + lockAttr + "]").forEach(function (button) {
      setLocked(button, false);
    });
  });
})();
