"use strict";

window.AmberAPI = (() => {
  const fallback = {
    version: 1,
    connectors: { connectedState: "connected", memoryBackend: "memory" },
    attachments: {
      maxBytes: 10 * 1024 * 1024,
      maxPerNode: 20,
      maxFilenameRunes: 180,
      imageMediaTypes: ["image/gif", "image/jpeg", "image/png", "image/webp"],
    },
    timeline: { titleMaxRunes: 140, summaryMaxRunes: 2000, sourceMaxRunes: 120, noteMaxRunes: 280 },
  };

  let runtime = JSON.parse(JSON.stringify(fallback));

  async function requestJSON(url, options = {}) {
    const headers = new Headers(options.headers || {});
    if (!headers.has("Accept")) headers.set("Accept", "application/json");
    if (options.body != null && !(options.body instanceof FormData) && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }

    const response = await fetch(url, { ...options, headers });
    if (response.status === 204) return null;

    const contentType = response.headers.get("Content-Type") || "";
    let payload;
    if (contentType.includes("application/json")) {
      try { payload = await response.json(); }
      catch {
        if (response.ok) throw new Error("API returned invalid JSON");
        payload = null;
      }
    } else {
      payload = await response.text().catch(() => "");
      if (response.ok && payload) throw new Error("API returned an unexpected content type");
    }
    if (!response.ok) {
      const message = payload && typeof payload === "object" ? payload.error : payload;
      const error = new Error(message || `API ${response.status}`);
      error.status = response.status;
      error.payload = payload;
      throw error;
    }
    return payload;
  }

  async function loadConfig() {
    const loaded = await requestJSON("/api/config");
    if (!loaded || loaded.version !== fallback.version) throw new Error("unsupported runtime config");
    runtime = loaded;
    applyConstraints();
    return runtime;
  }

  function applyConstraints(root = document) {
    const limits = runtime.timeline;
    setMaxLength(root, "#event-title", limits.titleMaxRunes);
    setMaxLength(root, "#event-summary", limits.summaryMaxRunes);
    setMaxLength(root, "#event-source", limits.sourceMaxRunes);
    root.querySelectorAll(".note-form textarea").forEach((element) => { element.maxLength = limits.noteMaxRunes; });
    const accept = runtime.attachments.imageMediaTypes.join(",");
    ["#subject-avatar-input", "#relation-photo-input"].forEach((selector) => {
      const element = root.querySelector(selector);
      if (element) element.accept = accept;
    });
  }

  function setMaxLength(root, selector, value) {
    const element = root.querySelector(selector);
    if (element) element.maxLength = value;
  }

  function getConfig() { return runtime; }
  function isConnected(state) { return state === runtime.connectors.connectedState; }
  function isPersistentBackend(id) { return Boolean(id && id !== runtime.connectors.memoryBackend); }
  function isImageMediaType(mediaType) { return runtime.attachments.imageMediaTypes.includes(mediaType); }

  return { requestJSON, loadConfig, applyConstraints, getConfig, isConnected, isPersistentBackend, isImageMediaType };
})();
