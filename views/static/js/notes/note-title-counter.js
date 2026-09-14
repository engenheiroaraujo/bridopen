window.BridopenPage.register("note-title-counter", function (ctx) {
  function chars(value) {
    return Array.from(value || "");
  }

  function setupCounter(root) {
    var input = root.querySelector("[data-note-title-input]");
    var counter = root.querySelector("[data-note-title-count]");
    if (!input || !counter) return;

    var max = parseInt(input.getAttribute("maxlength") || "50", 10);
    if (!Number.isFinite(max) || max <= 0) max = 50;

    function update() {
      var list = chars(input.value);
      if (list.length > max) {
        input.value = list.slice(0, max).join("");
        list = chars(input.value);
      }

      counter.textContent = list.length + "/" + max;
      root.classList.toggle("is-near-limit", list.length >= Math.ceil(max * 0.8));
      root.classList.toggle("is-at-limit", list.length >= max);
    }

    input.addEventListener("input", update);
    update();
  }

  function init() {
    document.querySelectorAll("[data-note-title-counter]").forEach(setupCounter);
  }

  init();
});
