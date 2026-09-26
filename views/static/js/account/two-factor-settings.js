window.BridopenPage.register("two-factor-settings", function (ctx) {
  var root = document.getElementById("twoFactorSettings");
  if (!root) return;

  var toggleBtn = document.getElementById("btnToggleTwoFactorSettings");
  var collapseBtn = document.getElementById("twoFactorCollapseBtn");
  var panel = document.getElementById("twoFactorSettingsPanel");
  var primaryBtn = document.getElementById("twoFactorPrimaryBtn");
  var currentStatus = document.getElementById("twoFactorCurrentStatus");
  var statusText = document.getElementById("twoFactorStatusText");
  var setupControls = document.getElementById("twoFactorSetupControls");
  var verificationArea = document.getElementById("twoFactorVerificationArea");
  var qrWrap = document.getElementById("twoFactorQRCodeWrap");
  var qrImage = document.getElementById("twoFactorQRCode");
  var manualWrap = document.getElementById("twoFactorManualSecretWrap");
  var manualSecret = document.getElementById("twoFactorManualSecret");
  var codeInput = document.getElementById("twoFactorCode");
  var totpApps = document.getElementById("twoFactorTOTPApps");
  var methodFieldset = document.getElementById("twoFactorMethodFieldset");
  var recoveryCodesPanel = document.getElementById("twoFactorRecoveryCodes");
  var recoveryCodesList = document.getElementById("twoFactorRecoveryCodeList");
  var copyRecoveryCodesBtn = document.getElementById("twoFactorCopyRecoveryCodes");
  var regenerateRecoveryCodesBtn = document.getElementById("twoFactorRegenerateRecoveryCodes");

  if (!panel || !primaryBtn) return;

  var csrfMeta = document.querySelector('meta[name="csrf-token"]');
  var csrfToken = csrfMeta ? csrfMeta.getAttribute("content") : "";
  var recoveryCodesText = [];
  var state = {
    active: parseBool(root.dataset.twoFactorActive),
    enabled: parseBool(root.dataset.twoFactorEnabled),
    method: root.dataset.twoFactorMethod || "",
    targetMethod: root.dataset.twoFactorTargetMethod || "",
    pendingMethod: "",
    pendingOperation: "",
    busy: false,
    releasePrimaryAction: null,
  };

  function parseBool(value) {
    return String(value).toLowerCase() === "true";
  }

  function selectedMethod() {
    if (state.targetMethod) return state.targetMethod;
    var selected = root.querySelector('input[name="two_factor_method"]:checked');
    return selected ? selected.value : "email";
  }

  function methodLabel(method) {
    if (method === "email") return "código por e-mail";
    if (method === "totp") return "aplicativo autenticador";
    return "verificação em duas etapas";
  }

  function selectedMethodIsEnabled() {
    return state.enabled && (!state.targetMethod || state.method === selectedMethod());
  }

  function anotherMethodIsEnabled() {
    return !!state.targetMethod && state.enabled && state.method !== selectedMethod();
  }

  function hasPendingChallenge() {
    return state.pendingOperation === "enable" || state.pendingOperation === "disable";
  }

  function statusLabel(status) {
    if (status && status.status_label) return status.status_label;
    if (!state.enabled) return "Desabilitado";
    if (state.method === "email") return "Habilitado por e-mail";
    if (state.method === "totp") return "Habilitado por aplicativo autenticador";
    return "Habilitado";
  }

  function setMessage(type, text) {
    text = String(text || "").trim();
    if (!text) return;
    if (window.BridopenToast && window.BridopenToast.show) {
      window.BridopenToast.show(text, type || "info");
      return;
    }
    if (window.showToast) {
      window.showToast(text, type || "info");
      return;
    }
    console[type === "error" ? "error" : "log"](text);
  }

  function hideRecoveryCodes() {
    recoveryCodesText = [];
    if (recoveryCodesList) recoveryCodesList.textContent = "";
    if (recoveryCodesPanel) recoveryCodesPanel.classList.add("hidden");
  }

  function renderRecoveryCodes(codes) {
    if (!recoveryCodesPanel || !recoveryCodesList || !Array.isArray(codes) || codes.length === 0) {
      hideRecoveryCodes();
      return;
    }
    recoveryCodesText = codes.slice();
    recoveryCodesList.textContent = "";
    recoveryCodesText.forEach(function (code) {
      var item = document.createElement("li");
      item.className = "rounded-lg border border-amber-200/80 bg-white/70 px-3 py-2 font-mono text-sm tracking-[0.08em] text-amber-950 dark:border-amber-900/50 dark:bg-slate-950/50 dark:text-amber-100";
      item.textContent = code;
      recoveryCodesList.appendChild(item);
    });
    recoveryCodesPanel.classList.remove("hidden");
  }

  function primaryButtonLabel() {
    if (anotherMethodIsEnabled()) return "Método indisponível";
    if (state.pendingOperation === "enable") return "Cancelar configuração";
    if (state.pendingOperation === "disable") return "Cancelar desativação";
    return selectedMethodIsEnabled() ? "Desabilitar" : "Habilitar";
  }

  function syncPrimaryButtonStyle() {
    var isCancelAction = hasPendingChallenge();
    primaryBtn.textContent = primaryButtonLabel();
    primaryBtn.classList.toggle("qn-app-modal__btn--primary", !isCancelAction);
    primaryBtn.classList.toggle("qn-app-modal__btn--secondary", isCancelAction);
  }

  function syncControlState() {
    var blockedByAnotherMethod = anotherMethodIsEnabled();
    syncPrimaryButtonStyle();
    primaryBtn.disabled = state.busy || blockedByAnotherMethod;
    primaryBtn.classList.toggle("opacity-70", state.busy || blockedByAnotherMethod);
    primaryBtn.classList.toggle("cursor-not-allowed", blockedByAnotherMethod);
    primaryBtn.setAttribute("aria-disabled", String(state.busy || blockedByAnotherMethod));
    if (regenerateRecoveryCodesBtn) {
      var canRegenerate = state.enabled && !state.busy && !hasPendingChallenge();
      regenerateRecoveryCodesBtn.classList.toggle("hidden", !state.enabled);
      regenerateRecoveryCodesBtn.disabled = !canRegenerate;
      regenerateRecoveryCodesBtn.classList.toggle("opacity-70", !canRegenerate);
      regenerateRecoveryCodesBtn.setAttribute("aria-disabled", String(!canRegenerate));
    }
  }

  function setBusy(isBusy) {
    if (isBusy) {
      if (window.BridopenActionLock && window.BridopenActionLock.begin) {
        state.releasePrimaryAction = window.BridopenActionLock.begin(primaryBtn);
      }
    } else if (state.releasePrimaryAction) {
      state.releasePrimaryAction();
      state.releasePrimaryAction = null;
    }
    state.busy = isBusy;
    syncControlState();
  }

  function resetSetupOutput() {
    state.pendingMethod = "";
    state.pendingOperation = "";
    if (verificationArea) verificationArea.classList.add("hidden");
    if (qrWrap) qrWrap.classList.add("hidden");
    if (qrImage) qrImage.removeAttribute("src");
    if (manualWrap) manualWrap.classList.add("hidden");
    if (manualSecret) manualSecret.textContent = "";
    if (codeInput) codeInput.value = "";
    if (methodFieldset) methodFieldset.classList.remove("hidden");
    hideRecoveryCodes();
  }

  function setPanelOpen(isOpen, shouldFocus) {
    panel.classList.toggle("hidden", !isOpen);
    if (toggleBtn) {
      toggleBtn.setAttribute("aria-expanded", String(isOpen));
      toggleBtn.classList.toggle("is-open", isOpen);
    }
    if (!isOpen) {
      resetSetupOutput();
      setMessage("", "");
      return;
    }
    renderState();
    if (!shouldFocus) return;
    if (state.enabled) {
      primaryBtn.focus();
      return;
    }
    var firstMethod = root.querySelector('input[name="two_factor_method"]');
    if (firstMethod) firstMethod.focus();
  }

  function renderState(status) {
    if (status) {
      state.active = !!status.active;
      state.enabled = !!status.enabled;
      state.method = status.method || "";
    }
    var label = statusLabel(status);
    var isBlockedByAnotherMethod = anotherMethodIsEnabled();
    if (currentStatus) currentStatus.textContent = label;
    if (statusText) statusText.textContent = label;
    if (setupControls) setupControls.classList.toggle("hidden", state.enabled);
    if (state.enabled) resetSetupOutput();
    if (!state.enabled) hideRecoveryCodes();
    updateMethodPanels();
    syncControlState();
    if (isBlockedByAnotherMethod) {
      setMessage(
        "info",
        "Sua conta já usa " + methodLabel(state.method) + ". Para configurar " + methodLabel(selectedMethod()) + ", desative o método ativo primeiro."
      );
    }
  }

  function updateMethodPanels() {
    var method = selectedMethod();
    if (totpApps) totpApps.classList.toggle("hidden", method !== "totp" || state.enabled);
  }

  function cancelPendingChallenge() {
    resetSetupOutput();
    if (setupControls) setupControls.classList.toggle("hidden", state.enabled);
    updateMethodPanels();
    setMessage("", "");
    syncControlState();
    primaryBtn.focus();
  }

  function postForm(url, fields) {
    var body = new URLSearchParams();
    Object.keys(fields || {}).forEach(function (key) {
      body.set(key, fields[key]);
    });
    return fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
        "X-CSRF-Token": csrfToken,
        Accept: "application/json",
      },
      body: body.toString(),
      // Abortado no unmount: sem isso a resposta chegaria depois da troca de
      // conteúdo e tentaria renderizar num DOM que já não existe.
      signal: ctx.signal,
    }).then(function (response) {
      return response
        .json()
        .catch(function () {
          return { ok: false, message: "Não foi possível processar a resposta." };
        })
        .then(function (data) {
          if (!response.ok) data.ok = false;
          return data;
        });
    }).catch(function () {
      return { ok: false, message: "Não foi possível concluir a ação. Tente novamente." };
    });
  }

  function startSetup() {
    if (anotherMethodIsEnabled()) {
      renderState();
      return;
    }
    if (!state.active) {
      setMessage("error", "Confirme sua conta antes de habilitar a verificação em duas etapas.");
      return;
    }
    resetSetupOutput();
    setBusy(true);
    postForm("/me/twofactor/start", { method: selectedMethod() })
      .then(function (data) {
        if (!data.ok) {
          setMessage("error", data.message || "Não foi possível iniciar a configuração.");
          if (data.status) renderState(data.status);
          return;
        }
        state.pendingMethod = data.setup ? data.setup.method : selectedMethod();
        state.pendingOperation = "enable";
        setMessage("success", data.message || "Informe o código para confirmar.");
        if (verificationArea) verificationArea.classList.remove("hidden");
        if (data.setup && data.setup.qr_code_data && qrImage && qrWrap) {
          qrImage.setAttribute("src", data.setup.qr_code_data);
          qrWrap.classList.remove("hidden");
        }
        if (data.setup && data.setup.manual_secret && manualSecret && manualWrap) {
          manualSecret.textContent = data.setup.manual_secret;
          manualWrap.classList.remove("hidden");
        }
        if (codeInput) codeInput.focus();
      })
      .finally(function () {
        setBusy(false);
      });
  }

  function verifySetup() {
    if (state.busy) return;
    var code = codeInput ? codeInput.value.replace(/\D/g, "") : "";
    if (code.length !== 6) {
      setMessage("error", "Informe um código de 6 dígitos.");
      if (codeInput) codeInput.focus();
      return;
    }
    if (state.pendingOperation === "disable") {
      verifyDisable(code);
      return;
    }
    setMessage("info", "Verificando código...");
    setBusy(true);
    postForm("/me/twofactor/verify", {
      method: state.pendingMethod || selectedMethod(),
      code: code,
    })
      .then(function (data) {
        if (!data.ok) {
          setMessage("error", data.message || "Código inválido.");
          if (codeInput) codeInput.focus();
          return;
        }
        setMessage("success", data.message || "Verificação em duas etapas ativada.");
        renderState(data.status);
        renderRecoveryCodes(data.recovery_codes);
      })
      .finally(function () {
        setBusy(false);
      });
  }

  function startDisable() {
    if (!selectedMethodIsEnabled()) {
      renderState();
      return;
    }
    resetSetupOutput();
    setBusy(true);
    postForm("/me/twofactor/disable/start", {})
      .then(function (data) {
        if (!data.ok) {
          setMessage("error", data.message || "Não foi possível iniciar a desativação.");
          if (data.status) renderState(data.status);
          return;
        }
        state.pendingOperation = "disable";
        state.pendingMethod = data.setup ? data.setup.method : state.method;
        if (setupControls) setupControls.classList.remove("hidden");
        if (methodFieldset) methodFieldset.classList.add("hidden");
        if (verificationArea) verificationArea.classList.remove("hidden");
        if (qrWrap) qrWrap.classList.add("hidden");
        if (manualWrap) manualWrap.classList.add("hidden");
        setMessage("success", data.message || "Informe o código para confirmar a desativação.");
        if (codeInput) codeInput.focus();
      })
      .finally(function () {
        setBusy(false);
      });
  }

  function verifyDisable(code) {
    setMessage("info", "Verificando código...");
    setBusy(true);
    postForm("/me/twofactor/disable/verify", { code: code })
      .then(function (data) {
        if (!data.ok) {
          setMessage("error", data.message || "Código inválido.");
          if (codeInput) codeInput.focus();
          return;
        }
        setMessage("success", data.message || "Verificação em duas etapas desativada.");
        resetSetupOutput();
        renderState(data.status);
      })
      .finally(function () {
        setBusy(false);
      });
  }

  function regenerateRecoveryCodes() {
    if (state.busy || !state.enabled || hasPendingChallenge()) return;
    setMessage("info", "Gerando novos códigos...");
    setBusy(true);
    postForm("/me/twofactor/recovery-codes/regenerate", {})
      .then(function (data) {
        if (!data.ok) {
          setMessage("error", data.message || "Não foi possível gerar novos códigos.");
          if (data.status) renderState(data.status);
          return;
        }
        setMessage("success", data.message || "Novos códigos de recuperação gerados.");
        renderState(data.status);
        renderRecoveryCodes(data.recovery_codes);
      })
      .finally(function () {
        setBusy(false);
      });
  }

  function copyRecoveryCodes() {
    if (!recoveryCodesText.length || !navigator.clipboard) {
      setMessage("error", "Não foi possível copiar os códigos automaticamente.");
      return;
    }
    navigator.clipboard.writeText(recoveryCodesText.join("\n")).then(
      function () {
        setMessage("success", "Códigos copiados.");
      },
      function () {
        setMessage("error", "Não foi possível copiar os códigos automaticamente.");
      }
    );
  }

  if (toggleBtn) {
    toggleBtn.addEventListener("click", function () {
      setPanelOpen(panel.classList.contains("hidden"), true);
    });
  }

  if (collapseBtn) {
    collapseBtn.addEventListener("click", function () {
      setPanelOpen(false, false);
      if (toggleBtn) toggleBtn.focus();
    });
  }

  primaryBtn.addEventListener("click", function () {
    if (hasPendingChallenge()) {
      cancelPendingChallenge();
      return;
    }
    if (anotherMethodIsEnabled()) {
      renderState();
      return;
    }
    if (selectedMethodIsEnabled()) {
      startDisable();
      return;
    }
    startSetup();
  });

  if (regenerateRecoveryCodesBtn) {
    regenerateRecoveryCodesBtn.addEventListener("click", regenerateRecoveryCodes);
  }

  if (copyRecoveryCodesBtn) {
    copyRecoveryCodesBtn.addEventListener("click", copyRecoveryCodes);
  }

  root.querySelectorAll('input[name="two_factor_method"]').forEach(function (input) {
    input.addEventListener("change", function () {
      resetSetupOutput();
      setMessage("", "");
      updateMethodPanels();
    });
  });

  if (codeInput) {
    codeInput.addEventListener("input", function () {
      codeInput.value = codeInput.value.replace(/\D/g, "").slice(0, 6);
      if (codeInput.value.length === 6 && !state.busy) {
        verifySetup();
      }
    });
    codeInput.addEventListener("keydown", function (e) {
      if (e.key === "Enter") {
        e.preventDefault();
        verifySetup();
      }
    });
  }

  ctx.on(document, "keydown", function (e) {
    if (!toggleBtn) return;
    if (e.key !== "Escape") return;
    if (panel.classList.contains("hidden")) return;
    setPanelOpen(false, false);
    toggleBtn.focus();
  });

  renderState();
});
