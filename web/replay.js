// The rollout replay on fleetml.dev: plays the lines of a recorded scripts/demo.sh run
// (assets/replay.json, written by web/build.py from web/demo-transcript.txt) into the
// terminal window, and updates the status strip from each line's "strip" values.
// Without JavaScript, with prefers-reduced-motion, or if the JSON does not load, the
// page keeps the static copy of the same lines that the build put there.
(function () {
  "use strict";
  var box = document.querySelector("[data-replay]");
  if (!box || !window.fetch || !("IntersectionObserver" in window)) { return; }
  if (window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches) { return; }

  var code = box.querySelector("[data-replay-lines]");
  var button = box.querySelector("[data-replay-button]");
  var cells = {};
  box.querySelectorAll("[data-strip]").forEach(function (d) { cells[d.getAttribute("data-strip")] = d; });

  var CHAR = 30, OUT = 600, STEP = 900, MOMENT = 1600; // milliseconds: about 20 seconds in all
  var data, run = 0;

  function wait(ms, id) {
    return new Promise(function (resolve, reject) {
      setTimeout(function () { return id === run ? resolve() : reject(); }, ms);
    });
  }

  // Every line is laid out from the start, invisible until it plays, so the
  // window keeps its size and nothing below it moves.
  function render() {
    code.textContent = "";
    return data.lines.map(function (l, i) {
      var line = document.createElement("span");
      line.className = "l " + l.kind + " pending";
      var shown = document.createElement("span");
      var rest = document.createElement("span");
      rest.className = "rest";
      rest.textContent = l.text;
      line.appendChild(shown); line.appendChild(rest);
      code.appendChild(line);
      if (i < data.lines.length - 1) { code.appendChild(document.createTextNode("\n")); }
      return { line: line, shown: shown, rest: rest, l: l };
    });
  }

  function setStrip(values, flash) {
    Object.keys(values).forEach(function (k) {
      var d = cells[k];
      if (!d) { return; }
      d.textContent = values[k];
      if (flash) { d.classList.remove("changed"); void d.offsetWidth; d.classList.add("changed"); }
    });
  }

  function blank() {
    var b = {};
    data.strip.forEach(function (k) { b[k] = "-"; });
    return b;
  }

  function play() {
    var id = ++run;
    var rows = render();
    setStrip(blank(), false);
    box.classList.add("playing");
    var p = Promise.resolve();
    rows.forEach(function (r) {
      var kind = r.l.kind;
      var before = kind === "step" ? STEP : (kind === "canary" || kind === "gate" || kind === "back") ? MOMENT : OUT;
      p = p.then(function () { return wait(before, id); }).then(function () {
        r.line.classList.remove("pending");
        if (kind !== "cmd") {
          r.shown.textContent = r.l.text; r.rest.textContent = "";
          return;
        }
        // A command is typed, after its prompt.
        var text = r.l.text, at = text.indexOf("$ ") + 2;
        var typed = Promise.resolve();
        for (var n = at; n <= text.length; n += 2) {
          (function (n) {
            typed = typed.then(function () {
              r.shown.textContent = text.slice(0, n); r.rest.textContent = text.slice(n);
              return wait(CHAR, id);
            });
          })(Math.min(n, text.length));
        }
        return typed.then(function () { r.shown.textContent = text; r.rest.textContent = ""; });
      }).then(function () { setStrip(r.l.strip, true); });
    });
    p.then(function () { box.classList.remove("playing"); }, function () { /* a newer run took over */ });
  }

  fetch("/assets/replay.json").then(function (r) { return r.ok ? r.json() : Promise.reject(); }).then(function (d) {
    data = d;
    code.setAttribute("aria-hidden", "true"); // screen readers get the full transcript below
    button.hidden = false;
    button.addEventListener("click", play);
    render();
    setStrip(blank(), false);
    var seen = new IntersectionObserver(function (entries) {
      if (entries.some(function (e) { return e.isIntersecting; })) { seen.disconnect(); play(); }
    }, { threshold: 0.35 });
    seen.observe(box);
  }).catch(function () { /* the static copy stays */ });
})();
