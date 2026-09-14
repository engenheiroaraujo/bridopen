window.BridopenPage.register("note-color-picker", function (ctx) {
  function normalizeHexColor(raw) {
    if (!raw) return null;
    var s = String(raw).trim();
    if (!s.startsWith("#")) s = "#" + s;
    var body = s.slice(1).toLowerCase();
    if (/^[0-9a-f]{3}$/.test(body)) {
      return "#" + body[0] + body[0] + body[1] + body[1] + body[2] + body[2];
    }
    if (/^[0-9a-f]{6}$/.test(body)) {
      return "#" + body;
    }
    return null;
  }

  function syncColorHexFields() {
    var form = document.getElementById("note-new-form");
    var picker = document.getElementById("color");
    var hexInput = document.getElementById("color-hex");
    if (!picker || !hexInput) return;

    function pickerToHex() {
      hexInput.value = picker.value.toLowerCase();
      hexInput.setCustomValidity("");
    }

    function commitHexFromInput() {
      var normalized = normalizeHexColor(hexInput.value);
      if (normalized) {
        picker.value = normalized;
        hexInput.value = normalized;
        hexInput.setCustomValidity("");
        return true;
      }
      return false;
    }

    picker.addEventListener("input", pickerToHex);
    picker.addEventListener("change", pickerToHex);

    hexInput.addEventListener("blur", function () {
      if (hexInput.value.trim() === "") {
        pickerToHex();
        hexInput.setCustomValidity("");
        return;
      }
      if (!commitHexFromInput()) {
        hexInput.setCustomValidity("Use #rgb ou #rrggbb (apenas 0-9 e a-f).");
      }
    });

    hexInput.addEventListener("input", function () {
      hexInput.setCustomValidity("");
      var normalized = normalizeHexColor(hexInput.value);
      if (normalized) {
        picker.value = normalized;
      }
    });

    if (form) {
      form.addEventListener("submit", function (e) {
        var normalized = normalizeHexColor(hexInput.value);
        if (!normalized) {
          e.preventDefault();
          hexInput.setCustomValidity(
            'Digite uma cor válida: #rgb ou #rrggbb (ex.: #3d5a47 ou #abc).'
          );
          hexInput.reportValidity();
          return;
        }
        hexInput.setCustomValidity("");
        picker.value = normalized;
        hexInput.value = normalized;
      });
    }

    var initial = normalizeHexColor(hexInput.value);
    if (initial) {
      picker.value = initial;
      hexInput.value = initial;
    } else {
      pickerToHex();
    }
  }

  syncColorHexFields();
});
