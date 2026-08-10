"use strict";

window.AmberRelations = (() => {
  const state = { initialized: false, loaded: false, snapshot: { nodes: [], edges: [], backend: "memory" }, cy: null, mode: "select", linkSource: null, draftPosition: null };
  const el = {};

  async function init() {
    if (!state.initialized) { cache(); bind(); state.initialized = true; }
    if (!state.loaded) await load();
    else { state.cy.resize(); state.cy.fit(undefined, 36); }
  }

  function cache() {
    ["relations-board", "relations-backend", "relation-select", "relation-add-node", "relation-connect", "relation-layout", "relation-search", "relations-empty", "relation-node-form", "relation-node-id", "relation-node-type", "relation-node-title", "relation-node-subtitle", "relation-node-details", "relation-node-risk", "relation-node-sources", "relation-node-delete", "relation-edge-form", "relation-edge-id", "relation-edge-source", "relation-edge-target", "relation-edge-from", "relation-edge-to", "relation-edge-label", "relation-edge-confidence", "relation-edge-confidence-value", "relation-edge-kind", "relation-edge-sources", "relation-edge-note", "relation-edge-delete"].forEach((id) => { el[id] = document.getElementById(id); });
  }

  function bind() {
    el["relation-select"].addEventListener("click", () => setMode("select"));
    el["relation-connect"].addEventListener("click", () => setMode(state.mode === "connect" ? "select" : "connect"));
    el["relation-add-node"].addEventListener("click", newNode);
    el["relation-layout"].addEventListener("click", autoLayout);
    el["relation-search"].addEventListener("input", filterBoard);
    el["relation-node-form"].addEventListener("submit", saveNode);
    el["relation-edge-form"].addEventListener("submit", saveEdge);
    el["relation-node-delete"].addEventListener("click", deleteNode);
    el["relation-edge-delete"].addEventListener("click", deleteEdge);
    el["relation-edge-confidence"].addEventListener("input", () => { el["relation-edge-confidence-value"].textContent = `${el["relation-edge-confidence"].value}%`; });
    document.addEventListener("amber:case-created", (event) => { state.snapshot = event.detail.relationships; state.loaded = true; if (state.initialized) build(); });
  }

  async function load() {
    try {
      const response = await fetch("/api/relationships", { headers: { Accept: "application/json" } });
      const payload = await response.json(); if (!response.ok) throw new Error(payload.error || `API ${response.status}`);
      state.snapshot = payload; state.loaded = true; build();
    } catch (error) { el["relations-empty"].hidden = false; el["relations-empty"].querySelector("p").textContent = `BOARD ERROR / ${error.message}`; }
  }

  function build() {
    if (state.cy) state.cy.destroy();
    const width = Math.max(el["relations-board"].clientWidth, 600), height = Math.max(el["relations-board"].clientHeight, 420);
    const elements = [
      ...state.snapshot.nodes.map((node) => ({ group: "nodes", classes: [node.primary ? "primary" : "", node.risk === "high" ? "high-risk" : ""].filter(Boolean).join(" "), data: { ...node, cardLabel: `[${node.type.toUpperCase()}]\n${node.title}${node.subtitle ? `\n${node.subtitle}` : ""}` }, position: { x: node.x * width, y: node.y * height } })),
      ...state.snapshot.edges.map((edge) => ({ group: "edges", data: { ...edge, threadLabel: `${edge.label.toUpperCase()} / ${edge.confidence}%`, source: edge.sourceId, target: edge.targetId } })),
    ];
    state.cy = cytoscape({ container: el["relations-board"], elements, layout: { name: "preset", fit: true, padding: 45 }, minZoom: .35, maxZoom: 2.2, wheelSensitivity: .18,
      style: [
        { selector: "node", style: { width: 174, height: 92, shape: "rectangle", "background-color": "#120d08", "border-width": 1, "border-color": "#6d4820", label: "data(cardLabel)", color: "#ffd98c", "font-family": "Cascadia Mono, Consolas, monospace", "font-size": 10, "text-wrap": "wrap", "text-max-width": 156, "text-valign": "center", "text-halign": "center", "overlay-opacity": 0 } },
        { selector: "node.primary", style: { width: 202, height: 112, "background-color": "#f5b94e", "border-width": 2, "border-color": "#ffd98c", color: "#0c0906", "font-size": 12 } },
        { selector: 'node[type = "account"]', style: { "border-color": "#6fd8c5" } },
        { selector: 'node[type = "evidence"]', style: { "border-color": "#ff735f" } },
        { selector: "node.high-risk", style: { "border-color": "#ff735f", "border-width": 2 } },
        { selector: "node:selected", style: { "border-color": "#ffd98c", "border-width": 3, "shadow-blur": 12, "shadow-color": "#f5b94e", "shadow-opacity": .5 } },
        { selector: "edge", style: { width: 2, "line-color": "#c77d2a", "target-arrow-color": "#c77d2a", "target-arrow-shape": "triangle", "curve-style": "bezier", label: "data(threadLabel)", color: "#d38b35", "font-family": "Cascadia Mono, Consolas, monospace", "font-size": 9, "text-background-color": "#080604", "text-background-opacity": 1, "text-background-padding": 4, "text-rotation": "autorotate", "overlay-opacity": 0 } },
        { selector: 'edge[kind = "critical"]', style: { "line-color": "#ff735f", "target-arrow-color": "#ff735f", color: "#ff735f" } },
        { selector: 'edge[kind = "evidence"]', style: { "line-color": "#6fd8c5", "target-arrow-color": "#6fd8c5", color: "#6fd8c5", "line-style": "dashed" } },
        { selector: "edge:selected", style: { width: 4, "z-index": 20 } },
        { selector: ".faded", style: { opacity: .12 } },
      ],
    });
    state.cy.on("tap", "node", (event) => handleNodeTap(event.target));
    state.cy.on("tap", "edge", (event) => { if (state.mode === "select") showEdge(event.target.data()); });
    state.cy.on("tap", (event) => { if (event.target === state.cy && state.mode === "select") showEmpty(); });
    state.cy.on("dragfree", "node", (event) => persistPosition(event.target));
    el["relations-backend"].textContent = (state.snapshot.backend || "memory").toUpperCase();
    if (!state.snapshot.nodes.length) showEmpty();
  }

  function handleNodeTap(node) {
    if (state.mode !== "connect") { showNode(node.data()); return; }
    if (!state.linkSource) { state.linkSource = node.id(); node.select(); boardMessage(`${node.data("title")} → SELECT TARGET`); return; }
    if (state.linkSource === node.id()) { state.linkSource = null; node.unselect(); boardMessage(I18n.t("relations.hint")); return; }
    const source = state.cy.getElementById(state.linkSource).data();
    state.linkSource = null; setMode("select");
    showEdge({ id: "", sourceId: source.id, targetId: node.id(), label: "", confidence: 50, kind: "standard", sourceIds: [], note: "" }, true);
  }

  function setMode(mode) {
    state.mode = mode; state.linkSource = null;
    el["relation-select"].classList.toggle("active", mode === "select"); el["relation-connect"].classList.toggle("active", mode === "connect");
    el["relations-board"].classList.toggle("connecting", mode === "connect");
    if (state.cy) state.cy.elements().unselect();
    boardMessage(mode === "connect" ? I18n.t("relations.connectHint") : I18n.t("relations.hint"));
  }

  function newNode() {
    const offset = (state.snapshot.nodes.length % 4) * .08;
    state.draftPosition = { x: Math.min(.86, .62 + offset), y: Math.min(.78, .24 + offset) };
    showNode({ id: "", type: "subject", title: "", subtitle: "", details: "", risk: "low", sourceIds: [], ...state.draftPosition, primary: false }, true);
    el["relation-node-title"].focus();
  }

  function showNode(node, draft = false) {
    el["relations-empty"].hidden = true; el["relation-edge-form"].hidden = true; el["relation-node-form"].hidden = false;
    el["relation-node-id"].value = node.id; el["relation-node-type"].value = node.type; el["relation-node-type"].disabled = node.primary;
    el["relation-node-title"].value = node.title; el["relation-node-subtitle"].value = node.subtitle || ""; el["relation-node-details"].value = node.details || ""; el["relation-node-risk"].value = node.risk || "low"; el["relation-node-sources"].value = (node.sourceIds || []).join(", ");
    el["relation-node-delete"].hidden = draft || node.primary;
  }

  function showEdge(edge, draft = false) {
    el["relations-empty"].hidden = true; el["relation-node-form"].hidden = true; el["relation-edge-form"].hidden = false;
    el["relation-edge-id"].value = edge.id; el["relation-edge-source"].value = edge.sourceId; el["relation-edge-target"].value = edge.targetId;
    el["relation-edge-from"].textContent = nodeTitle(edge.sourceId); el["relation-edge-to"].textContent = nodeTitle(edge.targetId); el["relation-edge-label"].value = edge.label || ""; el["relation-edge-confidence"].value = edge.confidence; el["relation-edge-confidence-value"].textContent = `${edge.confidence}%`; el["relation-edge-kind"].value = edge.kind || "standard"; el["relation-edge-sources"].value = (edge.sourceIds || []).join(", "); el["relation-edge-note"].value = edge.note || ""; el["relation-edge-delete"].hidden = draft;
  }

  function showEmpty() { el["relations-empty"].hidden = false; el["relation-node-form"].hidden = true; el["relation-edge-form"].hidden = true; }
  function boardMessage(text) { showEmpty(); el["relations-empty"].querySelector("p").textContent = text; }

  async function saveNode(event) {
    event.preventDefault(); const id = el["relation-node-id"].value; const existing = state.snapshot.nodes.find((node) => node.id === id);
    const payload = { id, type: el["relation-node-type"].value, title: el["relation-node-title"].value.trim(), subtitle: el["relation-node-subtitle"].value.trim(), details: el["relation-node-details"].value.trim(), risk: el["relation-node-risk"].value, sourceIds: csv(el["relation-node-sources"].value), x: existing?.x ?? state.draftPosition?.x ?? .5, y: existing?.y ?? state.draftPosition?.y ?? .5, primary: existing?.primary || false };
    const saved = await api(id ? `/api/relationships/nodes/${id}` : "/api/relationships/nodes", { method: id ? "PUT" : "POST", body: JSON.stringify(payload) });
    if (!saved) return; if (id) state.snapshot.nodes[state.snapshot.nodes.findIndex((node) => node.id === id)] = saved; else state.snapshot.nodes.push(saved); state.draftPosition = null; build(); state.cy.getElementById(saved.id).select(); showNode(saved);
  }

  async function saveEdge(event) {
    event.preventDefault(); const id = el["relation-edge-id"].value;
    const payload = { id, sourceId: el["relation-edge-source"].value, targetId: el["relation-edge-target"].value, label: el["relation-edge-label"].value.trim(), confidence: Number(el["relation-edge-confidence"].value), kind: el["relation-edge-kind"].value, sourceIds: csv(el["relation-edge-sources"].value), note: el["relation-edge-note"].value.trim() };
    const saved = await api(id ? `/api/relationships/edges/${id}` : "/api/relationships/edges", { method: id ? "PUT" : "POST", body: JSON.stringify(payload) });
    if (!saved) return; if (id) state.snapshot.edges[state.snapshot.edges.findIndex((edge) => edge.id === id)] = saved; else state.snapshot.edges.push(saved); build(); state.cy.getElementById(saved.id).select(); showEdge(saved);
  }

  async function deleteNode() {
    const id = el["relation-node-id"].value; if (!id || !window.confirm(I18n.t("relations.deleteNodeConfirm"))) return;
    if (!await api(`/api/relationships/nodes/${id}`, { method: "DELETE" }, true)) return;
    state.snapshot.nodes = state.snapshot.nodes.filter((node) => node.id !== id); state.snapshot.edges = state.snapshot.edges.filter((edge) => edge.sourceId !== id && edge.targetId !== id); build(); showEmpty();
  }

  async function deleteEdge() {
    const id = el["relation-edge-id"].value; if (!id || !window.confirm(I18n.t("relations.deleteEdgeConfirm"))) return;
    if (!await api(`/api/relationships/edges/${id}`, { method: "DELETE" }, true)) return;
    state.snapshot.edges = state.snapshot.edges.filter((edge) => edge.id !== id); build(); showEmpty();
  }

  async function persistPosition(cyNode) {
    const width = Math.max(el["relations-board"].clientWidth, 1), height = Math.max(el["relations-board"].clientHeight, 1), item = state.snapshot.nodes.find((node) => node.id === cyNode.id()); if (!item) return;
    item.x = clamp(cyNode.position("x") / width); item.y = clamp(cyNode.position("y") / height); await api(`/api/relationships/nodes/${item.id}`, { method: "PUT", body: JSON.stringify(item) });
  }

  function autoLayout() {
    if (!state.cy || state.cy.nodes().length < 2) return;
    const layout = state.cy.layout({ name: "concentric", fit: true, padding: 55, animate: true, animationDuration: 350, concentric: (node) => node.data("primary") ? 10 : 1, levelWidth: () => 2 });
    layout.on("layoutstop", () => state.cy.nodes().forEach((node) => persistPosition(node))); layout.run();
  }

  function filterBoard(event) {
    if (!state.cy) return; const query = event.target.value.trim().toLowerCase(); state.cy.elements().removeClass("faded"); if (!query) return;
    state.cy.nodes().forEach((node) => { const match = `${node.data("title")} ${node.data("subtitle")} ${node.data("type")}`.toLowerCase().includes(query); if (!match) { node.addClass("faded"); node.connectedEdges().addClass("faded"); } });
  }

  async function api(url, options, noContent = false) {
    try { const response = await fetch(url, { ...options, headers: { "Content-Type": "application/json", Accept: "application/json" } }); if (!response.ok) { const payload = await response.json(); throw new Error(payload.error || `API ${response.status}`); } return noContent ? true : response.json(); }
    catch (error) { boardMessage(`BOARD ERROR / ${error.message}`); return null; }
  }

  function nodeTitle(id) { return state.snapshot.nodes.find((node) => node.id === id)?.title || id; }
  function csv(value) { return value.split(",").map((item) => item.trim()).filter(Boolean); }
  function clamp(value) { return Math.max(0, Math.min(1, value)); }
  return { init, get loaded() { return state.loaded; } };
})();
