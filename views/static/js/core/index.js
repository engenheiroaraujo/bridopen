(function () {
  var doc = document.documentElement;

  function syncThemeToggleUi() {
    var dark = doc.classList.contains("dark");
    document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
      btn.setAttribute("aria-pressed", dark ? "true" : "false");
      btn.setAttribute("aria-label", dark ? "Habilitar tema claro" : "Habilitar tema escuro");
      var label = btn.querySelector("[data-theme-toggle-label]");
      if (label) label.textContent = dark ? "Tema claro" : "Tema escuro";
    });
  }

  document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var dark = doc.classList.toggle("dark");
      try {
        localStorage.setItem("qn-theme", dark ? "dark" : "light");
      } catch (e) {}
      syncThemeToggleUi();
    });
  });
  syncThemeToggleUi();

  /**
   * Destaca a seção alvo ao entrar numa página de conta. Depende do conteúdo,
   * então é módulo de página: precisa rodar de novo a cada navegação parcial,
   * senão o realce só aparece no primeiro carregamento.
   */
  window.BridopenPage.register("account-focus-section", function (ctx) {
    var container = document.querySelector("[data-account-focus-section]");
    if (!container) return;

    var sectionId = container.getAttribute("data-account-focus-section");
    if (!sectionId) return;

    var visibleModal = document.querySelector(".qn-app-modal.flex");
    if (visibleModal) return;

    var target = document.getElementById(sectionId);
    if (!target) return;

    var behavior = window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth";
    var frame = window.requestAnimationFrame(function () {
      target.scrollIntoView({ behavior: behavior, block: "start" });
      target.classList.add("qn-me-section--spotlight");
      ctx.timeout(function () {
        target.classList.remove("qn-me-section--spotlight");
      }, 900);
    });

    // Numa navegação rápida o frame poderia disparar já na página seguinte.
    ctx.cleanup(function () {
      window.cancelAnimationFrame(frame);
      target.classList.remove("qn-me-section--spotlight");
    });
  });

  window.BridopenPage.register("scroll-top", function (ctx) {
    document.querySelectorAll(".js-scroll-top").forEach(function (el) {
      ctx.on(el, "click", function (e) {
        e.preventDefault();
        window.scrollTo({ top: 0, behavior: "smooth" });
      });
    });
  });

  document.addEventListener(
    "click",
    function (e) {
      var btn = e.target.closest("button[data-password-target]");
      if (!btn) return;
      var inputId = btn.getAttribute("data-password-target");
      if (!inputId) return;
      var pwdInput = document.getElementById(inputId);
      if (!pwdInput) return;
      e.preventDefault();
      var show = pwdInput.type === "password";
      pwdInput.type = show ? "text" : "password";
      var isConfirm = inputId.indexOf("confirm") !== -1;
      btn.setAttribute("aria-pressed", show ? "true" : "false");
      btn.setAttribute(
        "aria-label",
        show
          ? isConfirm
            ? "Ocultar confirmação de senha"
            : "Ocultar senha"
          : isConfirm
            ? "Mostrar confirmação de senha"
            : "Mostrar senha"
      );
      var eye = btn.querySelector(".icon-eye");
      var eyeOff = btn.querySelector(".icon-eye-off");
      if (eye) eye.classList.toggle("hidden", show);
      if (eyeOff) eyeOff.classList.toggle("hidden", !show);
    },
    false
  );

  function manualEntryField(target) {
    if (!target || !target.closest) return null;
    return target.closest("[data-manual-entry-required]");
  }

  function preventManualEntryBypass(e) {
    var input = manualEntryField(e.target);
    if (!input) return;
    e.preventDefault();
  }

  document.addEventListener("paste", preventManualEntryBypass, true);
  document.addEventListener("drop", preventManualEntryBypass, true);
  document.addEventListener(
    "beforeinput",
    function (e) {
      if (e.inputType !== "insertFromPaste" && e.inputType !== "insertFromDrop") return;
      preventManualEntryBypass(e);
    },
    true
  );

  function syncAuthCaptcha(form) {
    if (!form) return true;

    var captchas = form.querySelectorAll("[data-auth-captcha]");
    if (!captchas.length) return true;

    var complete = true;
    captchas.forEach(function (captcha) {
      var captchaCheck = captcha.querySelector("#captcha_checked");
      var captchaPanel = captcha.querySelector("[data-captcha-panel]");
      var captchaAnswer = captcha.querySelector("#captcha_answer");

      if (!captchaCheck || !captchaPanel || !captchaAnswer) return;

      var checked = captchaCheck.checked;
      captchaPanel.hidden = !checked;
      captchaPanel.classList.toggle("hidden", !checked);
      captchaAnswer.disabled = !checked;
      captchaAnswer.required = checked;

      if (!checked) {
        captchaAnswer.value = "";
        captchaAnswer.setCustomValidity("");
        complete = false;
        return;
      }

      if (!captchaAnswer.value.trim()) {
        complete = false;
      }
    });

    return complete;
  }

  function bindAuthCaptchaForms() {
    document.querySelectorAll("form").forEach(function (form) {
      if (!form.querySelector("[data-auth-captcha]")) return;

      var submit = form.querySelector('button[type="submit"], input[type="submit"]');
      var handledByPasswordSync = !!form.querySelector("#reset-password-submit");

      function sync() {
        var complete = syncAuthCaptcha(form);
        if (handledByPasswordSync || !submit) return;
        submit.disabled = !complete;
        submit.setAttribute("aria-disabled", complete ? "false" : "true");
      }

      form.querySelectorAll("#captcha_checked, #captcha_answer").forEach(function (input) {
        ["input", "change", "keyup", "paste", "focus", "blur"].forEach(function (eventName) {
          input.addEventListener(eventName, sync);
        });
      });

      window.addEventListener("pageshow", sync);
      sync();
      window.setTimeout(sync, 100);
    });
  }

  bindAuthCaptchaForms();

  var resetPwdSubmit = document.getElementById("reset-password-submit");
  var resetPwdForm = resetPwdSubmit ? resetPwdSubmit.closest("form") : null;
  var resetPwd = resetPwdForm ? resetPwdForm.querySelector("#password") : null;
  var resetPwdConfirm = resetPwdForm ? resetPwdForm.querySelector("#password_confirm") : null;
  if (resetPwdForm && resetPwd && resetPwdConfirm && resetPwdSubmit) {
    function syncResetPasswordSubmit() {
      var password = resetPwd.value;
      var confirmation = resetPwdConfirm.value;
      var minLength = parseInt(resetPwd.getAttribute("minlength") || "6", 10);
      var captchaOk = syncAuthCaptcha(resetPwdForm);
      var ok = password.length >= minLength && confirmation.length >= minLength && confirmation === password && captchaOk;

      if (confirmation && confirmation !== password) {
        resetPwdConfirm.setCustomValidity("As senhas não conferem.");
      } else {
        resetPwdConfirm.setCustomValidity("");
      }

      resetPwdSubmit.disabled = !ok;
      resetPwdSubmit.setAttribute("aria-disabled", ok ? "false" : "true");
    }

    ["input", "change", "keyup", "paste", "focus", "blur"].forEach(function (eventName) {
      resetPwd.addEventListener(eventName, syncResetPasswordSubmit);
      resetPwdConfirm.addEventListener(eventName, syncResetPasswordSubmit);
    });

    resetPwdForm.querySelectorAll("#captcha_checked, #captcha_answer").forEach(function (input) {
      ["input", "change", "keyup", "paste", "focus", "blur"].forEach(function (eventName) {
        input.addEventListener(eventName, syncResetPasswordSubmit);
      });
    });

    resetPwdForm.addEventListener("submit", function (e) {
      syncResetPasswordSubmit();
      if (resetPwdSubmit.disabled) {
        e.preventDefault();
      }
    });

    window.addEventListener("pageshow", syncResetPasswordSubmit);
    syncResetPasswordSubmit();
    window.setTimeout(syncResetPasswordSubmit, 100);
    window.setTimeout(syncResetPasswordSubmit, 500);
  }

})();
