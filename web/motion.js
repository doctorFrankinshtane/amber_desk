"use strict";

window.AmberMotion = (() => {
  const preference = window.matchMedia("(prefers-reduced-motion: reduce)");
  const activeAnimations = new WeakMap();
  const typingRuns = new WeakMap();

  function reduced() {
    return preference.matches;
  }

  function cancel(element) {
    activeAnimations.get(element)?.cancel();
    activeAnimations.delete(element);
  }

  function play(element, keyframes, options) {
    if (!element?.animate || document.hidden) return null;
    cancel(element);
    const animation = element.animate(keyframes, options);
    activeAnimations.set(element, animation);
    const clear = () => { if (activeAnimations.get(element) === animation) activeAnimations.delete(element); };
    animation.addEventListener("finish", clear, { once: true });
    animation.addEventListener("cancel", clear, { once: true });
    return animation;
  }

  function reveal(element, { axis = "y", distance = 7, duration = 200, delay = 0 } = {}) {
    if (!element) return null;
    if (reduced()) {
      return play(element, [{ opacity: 0.65 }, { opacity: 1 }], {
        duration: 100,
        delay,
        easing: "cubic-bezier(0.23, 1, 0.32, 1)",
      });
    }
    const transform = axis === "x" ? `translateX(-${distance}px)` : `translateY(${distance}px)`;
    return play(element, [{ opacity: 0, transform }, { opacity: 1, transform: "translate(0, 0)" }], {
      duration,
      delay,
      easing: "steps(4, end)",
    });
  }

  function revealList(elements, { limit = 6, interval = 35, axis = "y" } = {}) {
    [...elements].slice(0, limit).forEach((element, index) => reveal(element, { axis, delay: index * interval }));
  }

  function pulse(element, tone = "ok") {
    if (!element) return null;
    element.dataset.motionTone = tone;
    return play(element, [{ opacity: 0.35 }, { opacity: 1 }], {
      duration: reduced() ? 100 : 160,
      easing: reduced() ? "cubic-bezier(0.23, 1, 0.32, 1)" : "steps(3, end)",
    });
  }

  function typeText(element, value, { tone = "ok" } = {}) {
    if (!element) return;
    const text = String(value ?? "");
    const previous = typingRuns.get(element);
    if (previous) {
      window.cancelAnimationFrame(previous.frame);
      if (previous.label === null) element.removeAttribute("aria-label"); else element.setAttribute("aria-label", previous.label);
      element.removeAttribute("aria-busy");
      typingRuns.delete(element);
    }
    element.dataset.motionTone = tone;
    if (reduced() || document.hidden || text.length < 2) {
      element.textContent = text;
      element.removeAttribute("aria-busy");
      pulse(element, tone);
      return;
    }

    const run = { frame: 0, startedAt: performance.now(), label: element.getAttribute("aria-label") };
    const duration = Math.min(280, Math.max(120, text.length * 10));
    typingRuns.set(element, run);
    element.setAttribute("aria-label", text);
    element.setAttribute("aria-busy", "true");
    element.textContent = "";

    const tick = (now) => {
      if (typingRuns.get(element) !== run) return;
      const progress = Math.min(1, (now - run.startedAt) / duration);
      const visible = Math.max(1, Math.ceil(text.length * progress));
      element.textContent = text.slice(0, visible);
      if (progress < 1) {
        run.frame = window.requestAnimationFrame(tick);
        return;
      }
      typingRuns.delete(element);
      element.removeAttribute("aria-busy");
      if (run.label === null) element.removeAttribute("aria-label"); else element.setAttribute("aria-label", run.label);
    };
    run.frame = window.requestAnimationFrame(tick);
  }

  function markGenerated(element) {
    if (!element) return;
    element.dataset.motionGenerated = "true";
    reveal(element, { axis: "x", duration: 200 });
    window.setTimeout(() => { delete element.dataset.motionGenerated; }, 240);
  }

  return { reduced, reveal, revealList, pulse, typeText, markGenerated };
})();
