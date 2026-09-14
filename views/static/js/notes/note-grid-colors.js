window.BridopenPage.register("note-grid-colors", function (ctx) {
  function applyNoteCardColors() {
    document.querySelectorAll("[data-note-color]").forEach(function (el) {
      var c = el.getAttribute("data-note-color");
      if (c) el.style.backgroundColor = c;
    });
  }

  applyNoteCardColors();
});
