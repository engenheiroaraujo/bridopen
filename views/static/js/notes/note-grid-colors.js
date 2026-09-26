window.BridopenPage.register("note-grid-colors", function (ctx) {
  function applyNoteCardColors() {
    document.querySelectorAll("[data-note-color]").forEach(function (el) {
      var c = el.getAttribute("data-note-color");
      if (c) {
        if (el.classList.contains("keep-note-card")) {
          el.style.setProperty("--note-accent", c);
        } else {
          el.style.backgroundColor = c;
        }
      }
    });
  }

  applyNoteCardColors();
});
