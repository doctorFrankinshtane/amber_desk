"use strict";

(() => {
  const frames = Object.freeze([
    [
      "   .----.   ",
      " .' o.   '. ",
      "/ .   o.   \\",
      "|o  .   o  |",
      "\\  o.   . /",
      " '.   o .'  ",
      "   '----'   ",
    ].join("\n"),
    [
      "   .----.   ",
      " .'  o.  '. ",
      "/  .   o.  \\",
      "| o  .   o |",
      "\\ .  o.   /",
      " '.    o.'  ",
      "   '----'   ",
    ].join("\n"),
    [
      "   .----.   ",
      " .'   o. '. ",
      "/   .   o. \\",
      "|  o  .   o|",
      "\\  .  o.  /",
      " '.o    .'  ",
      "   '----'   ",
    ].join("\n"),
    [
      "   .----.   ",
      " .'    o.'. ",
      "/.   .   o \\",
      "|o  o  .   |",
      "\\   .  o. /",
      " '. o    .' ",
      "   '----'   ",
    ].join("\n"),
    [
      "   .----.   ",
      " .'o    .'. ",
      "/o .   .   \\",
      "|  o  o  . |",
      "\\.   .  o /",
      " '.  o   .' ",
      "   '----'   ",
    ].join("\n"),
    [
      "   .----.   ",
      " .'.o    '. ",
      "/ o  .   . \\",
      "|   o  o  .|",
      "\\o .   .  /",
      " '.   o  .' ",
      "   '----'   ",
    ].join("\n"),
  ]);

  const motionPreference = window.matchMedia("(prefers-reduced-motion: reduce)");
  let element;
  let frameIndex = 0;
  let timer;

  function render() {
    element.textContent = frames[frameIndex];
  }

  function stop() {
    if (timer === undefined) return;
    window.clearInterval(timer);
    timer = undefined;
  }

  function start() {
    stop();
    if (document.hidden || motionPreference.matches) return;
    timer = window.setInterval(() => {
      frameIndex = (frameIndex + 1) % frames.length;
      render();
    }, 180);
  }

  function syncPlayback() {
    if (document.hidden || motionPreference.matches) {
      stop();
      return;
    }
    start();
  }

  function init() {
    element = document.getElementById("ascii-planet");
    if (!element) return;

    element.setAttribute("aria-live", "off");
    render();
    document.addEventListener("visibilitychange", syncPlayback);
    motionPreference.addEventListener("change", syncPlayback);
    start();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init, { once: true });
  } else {
    init();
  }
})();
