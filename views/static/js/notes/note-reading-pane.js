window.BridopenPage.register("note-reading-pane", function (ctx) {
  var root = document.querySelector(".bear-notebook");
  if (!root) return;
  var pane = root.querySelector("[data-note-reading]");
  var links = Array.from(root.querySelectorAll("[data-note-select]"));
  function select(link, focus) {
    var id = link.getAttribute("data-note-select");
    var template = root.querySelector('template[data-note-document="' + id + '"]');
    if (!template) return;
    pane.replaceChildren(template.content.cloneNode(true));
    links.forEach(function (item) {
      var active = item === link;
      item.closest(".keep-note-card").classList.toggle("bear-selected", active);
      if (active) item.setAttribute("aria-current", "true");
      else item.removeAttribute("aria-current");
    });
    if (focus) {
      root.classList.add("bear-reading-open");
      pane.querySelector("h2").focus({preventScroll: true});
    }
  }
  ctx.on(root, "click", function (event) {
    var link = event.target.closest("[data-note-select]");
    if (!link || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    select(link, true);
  });
  ctx.on(root.querySelector(".bear-back"), "click", function () {
    root.classList.remove("bear-reading-open");
    var active = root.querySelector('[data-note-select][aria-current="true"]');
    if (active) active.focus({preventScroll: true});
  });
  if (links.length) select(links[0], false);
});
