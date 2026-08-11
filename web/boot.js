"use strict";

document.documentElement.classList.add("boot-pending");
document.documentElement.setAttribute("aria-busy", "true");

window.__amberBootFallback = window.setTimeout(() => {
  document.documentElement.classList.remove("boot-pending");
  document.documentElement.removeAttribute("aria-busy");
}, 3000);
