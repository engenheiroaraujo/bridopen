/**
 * Ciclo de vida dos módulos de página.
 *
 * Os scripts de conteúdo são carregados uma única vez pelo shell e registram um
 * `mount`. A navegação parcial chama `unmountAll()` antes de trocar o conteúdo e
 * `mountAll()` depois, então cada módulo volta a procurar o próprio elemento na
 * página nova.
 *
 * Cada `mount` recebe um contexto com `on`, `timeout`, `interval` e `signal`.
 * Tudo que passar por ele é desfeito automaticamente no unmount — é assim que se
 * evita listener duplicado, timer acumulado e fetch respondendo depois que o
 * elemento já saiu do DOM. Listeners presos a elementos dentro do conteúdo
 * trocado morrem junto com o DOM e não precisam de cuidado especial; os que
 * ficam em `document`/`window` precisam, e é para eles que `ctx.on` existe.
 */
window.BridopenPage = (function () {
  var modules = [];
  var active = [];
  var booted = false;

  function createContext() {
    var controller = new AbortController();
    var timers = [];
    var intervals = [];
    var extras = [];

    return {
      controller: controller,
      ctx: {
        signal: controller.signal,
        /** Listener removido automaticamente no unmount. */
        on: function (target, type, handler, options) {
          if (!target) return;
          var opts = { signal: controller.signal };
          if (options && typeof options === "object") {
            Object.keys(options).forEach(function (key) {
              opts[key] = options[key];
            });
            opts.signal = controller.signal;
          } else if (options === true) {
            opts.capture = true;
          }
          target.addEventListener(type, handler, opts);
        },
        /** setTimeout cancelado automaticamente no unmount. */
        timeout: function (fn, ms) {
          var id = window.setTimeout(fn, ms);
          timers.push(id);
          return id;
        },
        clearTimeout: function (id) {
          window.clearTimeout(id);
        },
        /** setInterval cancelado automaticamente no unmount. */
        interval: function (fn, ms) {
          var id = window.setInterval(fn, ms);
          intervals.push(id);
          return id;
        },
        /** Registra uma limpeza extra, para o que não couber nos helpers. */
        cleanup: function (fn) {
          if (typeof fn === "function") extras.push(fn);
        },
      },
      dispose: function () {
        controller.abort();
        timers.forEach(function (id) {
          window.clearTimeout(id);
        });
        intervals.forEach(function (id) {
          window.clearInterval(id);
        });
        extras.forEach(function (fn) {
          try {
            fn();
          } catch (e) {
            /* uma limpeza que falha não pode impedir as demais */
          }
        });
      },
    };
  }

  function mountOne(module) {
    var scope = createContext();
    var extra;
    try {
      extra = module.mount(scope.ctx);
    } catch (e) {
      if (window.console && console.error) {
        console.error("Bridopen: falha ao montar módulo " + module.name, e);
      }
    }
    if (typeof extra === "function") scope.ctx.cleanup(extra);
    active.push(scope);
  }

  return {
    /**
     * Registra um módulo de página. Fora de uma navegação (carga direta), monta
     * na hora se o documento já estiver pronto — mesmo comportamento do IIFE
     * que estes scripts tinham antes.
     */
    register: function (name, mount) {
      var module = { name: name, mount: mount };
      modules.push(module);
      if (booted) mountOne(module);
    },
    mountAll: function () {
      booted = true;
      modules.forEach(mountOne);
    },
    unmountAll: function () {
      active.forEach(function (scope) {
        scope.dispose();
      });
      active = [];
    },
  };
})();

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", function () {
    window.BridopenPage.mountAll();
  });
} else {
  window.BridopenPage.mountAll();
}
