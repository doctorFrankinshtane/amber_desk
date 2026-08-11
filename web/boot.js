"use strict";

document.documentElement.classList.add("boot-pending");
document.documentElement.setAttribute("aria-busy", "true");

window.AmberBoot = (() => {
  const root = document.documentElement;
  const startedAt = performance.now();
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
  const slots = { core: 160, api: 620, vault: 1080, case: 1540, workspace: 2000 };
  const order = Object.keys(slots);
  const scheduled = [];
  let finishing = null;

  function delay(milliseconds) {
    return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
  }

  function reveal() {
    window.clearTimeout(window.__amberBootFallback);
    root.classList.remove("boot-revealing", "boot-pending");
    root.removeAttribute("aria-busy");
  }

  function report(id, state, label) {
    const wait = reduced.matches ? 0 : Math.max(0, (slots[id] || 0) - (performance.now() - startedAt));
    const update = delay(wait).then(() => {
      const row = document.querySelector(`[data-boot-step="${id}"]`);
      if (!row) return;
      row.dataset.state = state;
      row.querySelector("b").textContent = label;
      const progress = document.getElementById("boot-progress-value");
      if (progress) progress.style.transform = `scaleX(${(order.indexOf(id) + 1) / order.length})`;
    });
    scheduled.push(update);
    return update;
  }

  function finish() {
    if (finishing) return finishing;
    finishing = (async () => {
      await Promise.all(scheduled);
      if (reduced.matches) { reveal(); return; }
      await delay(Math.max(0, 2320 - (performance.now() - startedAt)));
      root.classList.add("boot-revealing");
      await delay(180);
      reveal();
    })();
    return finishing;
  }

  window.__amberBootFallback = window.setTimeout(reveal, 4500);
  return { report, finish };
})();
