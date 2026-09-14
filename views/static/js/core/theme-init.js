(function () {
  var key = "qn-theme";
  var stored = null;
  try {
    stored = localStorage.getItem(key);
  } catch (e) {
    stored = null;
  }
  if (stored === "dark" || (!stored && window.matchMedia("(prefers-color-scheme: dark)").matches)) {
    document.documentElement.classList.add("dark");
  }
})();
