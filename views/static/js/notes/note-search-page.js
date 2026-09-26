window.BridopenPage.register("note-search-page", function (ctx) {
  var form = document.querySelector("[data-note-search-form]");
  if (!form) return;

  var searchInput = form.querySelector("[data-note-search-input]");
  var submitTimer = null;

  function submitForm() {
    if (form.requestSubmit) {
      form.requestSubmit();
      return;
    }
    form.submit();
  }

  function scheduleSubmit() {
    // ctx.timeout cancela o debounce pendente se o utilizador navegar antes de
    // ele disparar — sem isso a busca submeteria já na página seguinte.
    ctx.clearTimeout(submitTimer);
    submitTimer = ctx.timeout(submitForm, 450);
  }

  function focusSearch() {
    if (!searchInput) return;
    searchInput.focus();
    searchInput.select();
  }

  function isTypingTarget(target) {
    if (!target) return false;
    var tag = target.tagName ? target.tagName.toLowerCase() : "";
    return tag === "input" || tag === "textarea" || tag === "select" || target.isContentEditable;
  }

  function highlightTerms() {
    if (!searchInput || !searchInput.value.trim()) return;
    var terms = searchInput.value
      .trim()
      .split(/\s+/)
      .filter(function (term) {
        var value = term.replace(/^["']|["']$/g, "");
        if (value.length < 2) return false;
        if (value.charAt(0) === "#") return false;
        if (/^(cor|color):/i.test(value)) return false;
        if (/^(fixada|fixadas|pinned)$/i.test(value)) return false;
        return true;
      })
      .slice(0, 5);
    if (!terms.length) return;

    var pattern = new RegExp("(" + terms.map(escapeRegExp).join("|") + ")", "gi");
    document.querySelectorAll("[data-note-highlight]").forEach(function (node) {
      var text = node.textContent || "";
      if (!pattern.test(text)) return;
      pattern.lastIndex = 0;

      var fragment = document.createDocumentFragment();
      var lastIndex = 0;
      text.replace(pattern, function (match, _term, offset) {
        if (offset > lastIndex) {
          fragment.appendChild(document.createTextNode(text.slice(lastIndex, offset)));
        }
        var mark = document.createElement("mark");
        mark.className = "rounded bg-amber-200/80 px-0.5 text-inherit dark:bg-amber-400/30";
        mark.textContent = match;
        fragment.appendChild(mark);
        lastIndex = offset + match.length;
        return match;
      });
      if (lastIndex < text.length) {
        fragment.appendChild(document.createTextNode(text.slice(lastIndex)));
      }
      node.textContent = "";
      node.appendChild(fragment);
    });
  }

  function escapeRegExp(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  if (searchInput) {
    searchInput.addEventListener("input", scheduleSubmit);
  }

  form.querySelectorAll("[data-note-auto-submit]").forEach(function (control) {
    control.addEventListener("change", submitForm);
  });

  // Atalhos "/" e Ctrl/Cmd+K vivem em document: passam por ctx.on para serem
  // removidos ao sair da página, evitando listener duplicado a cada navegação.
  ctx.on(document, "keydown", function (event) {
    if (event.key === "/" && !isTypingTarget(event.target)) {
      event.preventDefault();
      focusSearch();
      return;
    }
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      focusSearch();
    }
  });

  highlightTerms();
});
