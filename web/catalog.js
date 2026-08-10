"use strict";

window.AmberCatalog = (() => {
  const state = { initialized: false, snapshot: null, category: "all", pricing: "all", status: "all", query: "", selectedID: null };
  const el = {};

  async function init() {
    if (!state.initialized) {
      cache();
      bind();
      state.initialized = true;
    }
    if (state.snapshot) return;
    setLoading("LOADING LOCAL CATALOG...");
    try {
      const response = await fetch("/api/catalog", { headers: { Accept: "application/json" } });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || `API ${response.status}`);
      state.snapshot = payload;
      state.selectedID = payload.tools[0]?.id || null;
      populateFilters();
      renderCategories();
      render();
      AmberMotion.revealList(el["catalog-tools"].children, { limit: 5, axis: "x" });
      AmberMotion.reveal(el["catalog-detail"], { axis: "x", duration: 200 });
    } catch (error) {
      setLoading(`CATALOG ERROR / ${error.message}`);
    }
  }

  function cache() {
    ["catalog-search", "catalog-pricing", "catalog-status", "catalog-count", "catalog-visible", "catalog-categories", "catalog-tools", "catalog-detail"].forEach((id) => { el[id] = document.getElementById(id); });
  }

  function bind() {
    el["catalog-search"].addEventListener("input", (event) => { state.query = event.target.value.trim().toLowerCase(); render(); });
    el["catalog-pricing"].addEventListener("change", (event) => { state.pricing = event.target.value; render(); });
    el["catalog-status"].addEventListener("change", (event) => { state.status = event.target.value; render(); });
    el["catalog-categories"].addEventListener("click", (event) => {
      const button = event.target.closest("button[data-category]");
      if (!button) return;
      state.category = button.dataset.category;
      renderCategories();
      render();
    });
    el["catalog-tools"].addEventListener("click", (event) => {
      const button = event.target.closest("button[data-tool-id]");
      if (!button) return;
      state.selectedID = button.dataset.toolId;
      renderTools(filteredTools());
      renderDetail();
    });
  }

  function populateFilters() {
    const pricing = uniqueValues(state.snapshot.tools, "pricing");
    const statuses = uniqueValues(state.snapshot.tools, "status");
    el["catalog-pricing"].append(...pricing.map((value) => option(value)));
    el["catalog-status"].append(...statuses.map((value) => option(value)));
    el["catalog-count"].textContent = `${String(state.snapshot.tools.length).padStart(4, "0")} TOOLS / ${state.snapshot.metadata.version.slice(0, 7)}`;
  }

  function renderCategories() {
    if (!state.snapshot) return;
    const total = state.snapshot.tools.length;
    const items = [{ id: "all", name: I18n.t("common.all"), toolCount: total }, ...state.snapshot.categories];
    el["catalog-categories"].replaceChildren(...items.map((category) => {
      const button = node("button", { type: "button", "data-category": category.id, class: state.category === category.id ? "active" : "" });
      button.append(node("span", {}, category.name), node("b", {}, String(category.toolCount).padStart(3, "0")));
      return button;
    }));
  }

  function render() {
    if (!state.snapshot) return;
    const tools = filteredTools();
    if (!tools.some((tool) => tool.id === state.selectedID)) state.selectedID = tools[0]?.id || null;
    el["catalog-visible"].textContent = String(tools.length).padStart(4, "0");
    renderTools(tools);
    renderDetail();
  }

  function filteredTools() {
    return state.snapshot.tools.filter((tool) => {
      const category = state.snapshot.categories.find((item) => item.id === state.category)?.name;
      const categoryMatch = state.category === "all" || tool.path[0] === category;
      const pricingMatch = state.pricing === "all" || tool.pricing === state.pricing;
      const statusMatch = state.status === "all" || tool.status === state.status;
      const text = [tool.name, tool.description, tool.bestFor, tool.input, tool.output, ...tool.path].join(" ").toLowerCase();
      return categoryMatch && pricingMatch && statusMatch && (!state.query || text.includes(state.query));
    });
  }

  function renderTools(tools) {
    if (!tools.length) {
      el["catalog-tools"].replaceChildren(node("p", { class: "catalog-placeholder" }, I18n.t("catalog.empty")));
      return;
    }
    el["catalog-tools"].replaceChildren(...tools.map((tool) => {
      const button = node("button", { type: "button", class: `catalog-tool${tool.id === state.selectedID ? " active" : ""}${tool.deprecated ? " deprecated" : ""}`, "data-tool-id": tool.id });
      const copy = node("span");
      copy.append(node("strong", {}, tool.name), node("small", {}, tool.path.slice(1).join(" / ") || tool.path[0]));
      const meta = node("span", { class: "catalog-tool-meta" });
      meta.append(node("b", { "data-opsec": tool.opsec || "unknown" }, (tool.opsec || "N/A").toUpperCase()), node("small", {}, (tool.pricing || "unknown").toUpperCase()));
      button.append(copy, meta);
      return button;
    }));
  }

  function renderDetail() {
    const tool = state.snapshot?.tools.find((item) => item.id === state.selectedID);
    if (!tool) {
      el["catalog-detail"].replaceChildren(node("p", { class: "catalog-placeholder" }, I18n.t("catalog.select")));
      return;
    }
    const fragment = document.createDocumentFragment();
    fragment.append(node("p", { class: "catalog-path" }, tool.path.join(" / ")), node("h3", {}, tool.name));
    const badges = node("div", { class: "catalog-badges" });
    [tool.status, tool.pricing, tool.opsec && `OPSEC:${tool.opsec}`, tool.localInstall && "LOCAL", tool.api && "API", tool.registration && "REG", tool.googleDork && "DORK", tool.insecureUrl && "HTTP", tool.deprecated && "DEPRECATED", ...tool.badges].filter(Boolean).forEach((value) => badges.append(node("span", {}, String(value).toUpperCase())));
    fragment.append(badges);
    appendSection(fragment, I18n.t("catalog.description"), tool.description || I18n.t("catalog.noDescription"));
    appendSection(fragment, I18n.t("catalog.bestFor"), tool.bestFor);
    if (tool.input || tool.output) appendSection(fragment, I18n.t("catalog.io"), `${tool.input || "--"} -> ${tool.output || "--"}`);
    appendSection(fragment, I18n.t("catalog.opsec"), tool.opsecNote || I18n.t("catalog.noOpsec"), "opsec-section");
    const actions = node("div", { class: "catalog-actions" });
    const open = node("a", { href: tool.url, target: "_blank", rel: "noopener noreferrer", referrerpolicy: "no-referrer" }, I18n.t("catalog.open"));
    const log = node("button", { type: "button" }, I18n.t("catalog.log"));
    log.addEventListener("click", () => logTool(tool, log));
    actions.append(log, open);
    fragment.append(actions, node("p", { class: "catalog-source-url" }, tool.url));
    el["catalog-detail"].replaceChildren(fragment);
  }

  async function logTool(tool, button) {
    button.disabled = true;
    try {
      const now = new Date();
      const response = await fetch("/api/timeline/events", { method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify({ occurredAt: now.toISOString(), time: now.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false }), date: now.toLocaleDateString("en-GB", { day: "2-digit", month: "short" }).toUpperCase(), type: "tool", title: `Tool selected: ${tool.name}`, summary: `${tool.path.join(" / ")}. ${tool.bestFor || tool.description || "OSINT source selected for investigation."}`, source: "OSINT Framework", sourceUrl: tool.url, confidence: 50, status: "pending", fingerprint: `CATALOG:${state.snapshot.metadata.version.slice(0, 12)}:${tool.id}`, indicators: [tool.url], notes: [] }) });
      const payload = await response.json();
      if (!response.ok) throw new Error(payload.error || `API ${response.status}`);
      AmberMotion.typeText(button, I18n.t("catalog.logged"));
      document.dispatchEvent(new CustomEvent("amber:timeline-changed", { detail: payload }));
    } catch (error) {
      AmberMotion.typeText(button, `ERROR / ${error.message}`, { tone: "error" });
    } finally {
      window.setTimeout(() => { button.disabled = false; button.textContent = I18n.t("catalog.log"); }, 1800);
    }
  }

  function appendSection(parent, title, text, className = "") {
    if (!text) return;
    const section = node("section", { class: className });
    section.append(node("h4", {}, title), node("p", {}, text));
    parent.append(section);
  }

  function setLoading(message) { const placeholder = node("p", { class: "catalog-placeholder" }); el["catalog-tools"].replaceChildren(placeholder); AmberMotion.typeText(placeholder, message); }
  function uniqueValues(items, field) { return [...new Set(items.map((item) => item[field]).filter(Boolean))].sort(); }
  function option(value) { return node("option", { value }, value.toUpperCase()); }
  function node(tag, attributes = {}, text = "") { const element = document.createElement(tag); Object.entries(attributes).forEach(([name, value]) => { if (name === "class") element.className = value; else element.setAttribute(name, value); }); if (text !== "") element.textContent = text; return element; }

  return { init, renderCategories, render, get loaded() { return Boolean(state.snapshot); } };
})();
