"use strict";

window.AmberRelations = (() => {
  const state = { initialized: false, loaded: false, snapshot: { nodes: [], edges: [], backend: AmberAPI.getConfig().connectors.memoryBackend }, cy: null, mode: "select", linkSource: null, draftPosition: null, attachments: new Map(), attachmentLayer: null, coverLayer: null, selectedNodeID: "", sherlock: { status: null, scan: null, source: null, selected: new Set(), stream: null } };
  const el = {};

  async function init() {
    if (!state.initialized) { cache(); bind(); state.initialized = true; }
    if (!state.loaded) await load();
    else { state.cy.resize(); state.cy.fit(undefined, 36); }
  }

  async function refresh() {
    state.loaded = false;
    state.snapshot = { nodes: [], edges: [], backend: AmberAPI.getConfig().connectors.memoryBackend };
    state.linkSource = null;
    if (state.initialized) await load();
  }

  function cache() {
    ["relations-board", "relations-backend", "relation-select", "relation-add-node", "relation-connect", "relation-layout", "relation-search", "relations-empty", "relation-node-form", "relation-node-id", "relation-node-type", "relation-node-title", "relation-node-subtitle", "relation-node-details", "relation-node-risk", "relation-node-sources", "relation-node-delete", "relation-attachments", "relation-attachment-input", "relation-attachment-add", "relation-photo-input", "relation-photo-add", "relation-attachment-status", "relation-attachment-list", "relation-attachment-count", "relation-edge-form", "relation-edge-id", "relation-edge-source", "relation-edge-target", "relation-edge-from", "relation-edge-to", "relation-edge-label", "relation-edge-confidence", "relation-edge-confidence-value", "relation-edge-kind", "relation-edge-sources", "relation-edge-note", "relation-edge-delete", "relation-sherlock", "sherlock-runtime", "sherlock-open", "sherlock-launch", "sherlock-username", "sherlock-message", "sherlock-prepare", "sherlock-console", "sherlock-checked", "sherlock-claimed", "sherlock-errors", "sherlock-state", "sherlock-filter", "sherlock-search", "sherlock-log", "sherlock-results", "sherlock-cancel", "sherlock-import", "sherlock-confirm", "sherlock-confirm-username", "sherlock-confirm-runtime", "sherlock-confirm-start"].forEach((id) => { el[id] = document.getElementById(id); });
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
    el["relation-attachment-add"].addEventListener("click", () => el["relation-attachment-input"].click());
    el["relation-photo-add"].addEventListener("click", () => el["relation-photo-input"].click());
    el["relation-attachment-input"].addEventListener("change", uploadAttachments);
    el["relation-photo-input"].addEventListener("change", uploadAttachments);
    el["relation-edge-delete"].addEventListener("click", deleteEdge);
    el["sherlock-open"].addEventListener("click", toggleSherlockPanel);
    el["sherlock-prepare"].addEventListener("click", prepareSherlockScan);
    el["sherlock-confirm-start"].addEventListener("click", startSherlockScan);
    el["sherlock-cancel"].addEventListener("click", cancelSherlockScan);
    el["sherlock-import"].addEventListener("click", importSherlockResults);
    el["sherlock-filter"].addEventListener("change", renderSherlockResults);
    el["sherlock-search"].addEventListener("input", renderSherlockResults);
    el["sherlock-results"].addEventListener("change", toggleSherlockResult);
    document.addEventListener("amber:relationships-changed", (event) => { if (event.detail?.source === "dossier-avatar") refresh(); });
    el["relation-edge-confidence"].addEventListener("input", () => { el["relation-edge-confidence-value"].textContent = `${el["relation-edge-confidence"].value}%`; });
    document.addEventListener("amber:case-created", (event) => { state.snapshot = event.detail.relationships; state.loaded = true; if (state.initialized) build(); });
  }

  async function load() {
    try {
      const payload = await AmberAPI.requestJSON("/api/relationships");
      state.snapshot = payload; state.loaded = true; build();
    } catch (error) { el["relations-empty"].hidden = false; el["relations-empty"].querySelector("p").textContent = `BOARD ERROR / ${error.message}`; }
  }

  function build() {
    if (state.cy) state.cy.destroy();
    const width = Math.max(el["relations-board"].clientWidth, 600), height = Math.max(el["relations-board"].clientHeight, 420);
    const elements = [
      ...state.snapshot.nodes.map((node) => ({ group: "nodes", classes: [node.primary ? "primary" : "", node.risk === "high" ? "high-risk" : "", node.coverAttachmentId ? "has-cover" : ""].filter(Boolean).join(" "), data: { ...node, cardLabel: `[${node.type.toUpperCase()}]\n${node.title}${node.subtitle ? `\n${node.subtitle}` : ""}` }, position: { x: node.x * width, y: node.y * height } })),
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
        { selector: "node.has-cover", style: { "text-opacity": 0 } },
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
    state.cy.on("render position pan zoom", syncBoardOverlays);
    renderCoverCards();
    renderAttachmentStacks();
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
    el["relation-attachments"].hidden = draft;
    state.selectedNodeID = node.id;
    el["relation-sherlock"].hidden = draft;
    if (!draft && state.sherlock.source?.id !== node.id) resetSherlockConsole(node);
    if (draft) { el["relation-attachment-list"].replaceChildren(); el["relation-attachment-status"].textContent = ""; }
    else loadAttachments(node.id);
  }

  function showEdge(edge, draft = false) {
    state.selectedNodeID = "";
    el["relations-empty"].hidden = true; el["relation-node-form"].hidden = true; el["relation-edge-form"].hidden = false;
    el["relation-edge-id"].value = edge.id; el["relation-edge-source"].value = edge.sourceId; el["relation-edge-target"].value = edge.targetId;
    el["relation-edge-from"].textContent = nodeTitle(edge.sourceId); el["relation-edge-to"].textContent = nodeTitle(edge.targetId); el["relation-edge-label"].value = edge.label || ""; el["relation-edge-confidence"].value = edge.confidence; el["relation-edge-confidence-value"].textContent = `${edge.confidence}%`; el["relation-edge-kind"].value = edge.kind || "standard"; el["relation-edge-sources"].value = (edge.sourceIds || []).join(", "); el["relation-edge-note"].value = edge.note || ""; el["relation-edge-delete"].hidden = draft;
  }

  function showEmpty() { state.selectedNodeID = ""; el["relations-empty"].hidden = false; el["relation-node-form"].hidden = true; el["relation-edge-form"].hidden = true; }
  function boardMessage(text) { showEmpty(); AmberMotion.typeText(el["relations-empty"].querySelector("p"), text, { tone: text.includes("ERROR") ? "error" : "ok" }); }

  async function saveNode(event) {
    event.preventDefault(); const id = el["relation-node-id"].value; const existing = state.snapshot.nodes.find((node) => node.id === id);
    const payload = { id, type: el["relation-node-type"].value, title: el["relation-node-title"].value.trim(), subtitle: el["relation-node-subtitle"].value.trim(), details: el["relation-node-details"].value.trim(), risk: el["relation-node-risk"].value, sourceIds: csv(el["relation-node-sources"].value), x: existing?.x ?? state.draftPosition?.x ?? .5, y: existing?.y ?? state.draftPosition?.y ?? .5, primary: existing?.primary || false, coverAttachmentId: existing?.coverAttachmentId || "", attachmentCount: existing?.attachmentCount || 0 };
    const saved = await api(id ? `/api/relationships/nodes/${id}` : "/api/relationships/nodes", { method: id ? "PUT" : "POST", body: JSON.stringify(payload) });
    if (!saved) return; if (id) state.snapshot.nodes[state.snapshot.nodes.findIndex((node) => node.id === id)] = saved; else state.snapshot.nodes.push(saved); state.draftPosition = null; build(); const savedNode = state.cy.getElementById(saved.id); savedNode.select(); showNode(saved);
    if (!id && !AmberMotion.reduced()) { savedNode.style("opacity", 0); savedNode.animate({ style: { opacity: 1 } }, { duration: 200, easing: "cubic-bezier(0.23, 1, 0.32, 1)" }); }
  }

  async function saveEdge(event) {
    event.preventDefault(); const id = el["relation-edge-id"].value;
    const payload = { id, sourceId: el["relation-edge-source"].value, targetId: el["relation-edge-target"].value, label: el["relation-edge-label"].value.trim(), confidence: Number(el["relation-edge-confidence"].value), kind: el["relation-edge-kind"].value, sourceIds: csv(el["relation-edge-sources"].value), note: el["relation-edge-note"].value.trim() };
    const saved = await api(id ? `/api/relationships/edges/${id}` : "/api/relationships/edges", { method: id ? "PUT" : "POST", body: JSON.stringify(payload) });
    if (!saved) return; if (id) state.snapshot.edges[state.snapshot.edges.findIndex((edge) => edge.id === id)] = saved; else state.snapshot.edges.push(saved); build(); const savedEdge = state.cy.getElementById(saved.id); savedEdge.select(); showEdge(saved);
    if (!id && !AmberMotion.reduced()) { savedEdge.style("opacity", 0); savedEdge.animate({ style: { opacity: 1 } }, { duration: 200, easing: "cubic-bezier(0.23, 1, 0.32, 1)" }); }
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

  async function loadAttachments(nodeID, animate = false) {
    el["relation-attachment-status"].textContent = "LOADING...";
    const items = await api(`/api/relationships/nodes/${encodeURIComponent(nodeID)}/attachments`, { method: "GET" });
    if (!items || el["relation-node-id"].value !== nodeID) return;
    state.attachments.set(nodeID, items);
    updateAttachmentCount(nodeID, items.length);
    renderAttachmentList(nodeID, items, animate);
  }

  function renderAttachmentList(nodeID, items, animate = false) {
    const node = state.snapshot.nodes.find((entry) => entry.id === nodeID);
    el["relation-attachment-status"].textContent = items.length ? "" : "NO DOCUMENTS";
    el["relation-attachment-count"].textContent = `${items.length} / 20`;
    el["relation-attachment-add"].disabled = items.length >= 20;
    el["relation-photo-add"].disabled = items.length >= 20;
    el["relation-attachment-list"].replaceChildren(...items.map((item) => {
      const row = document.createElement("div"); row.className = "relation-attachment-item";
      const preview = isImageAttachment(item) ? document.createElement("img") : document.createElement("span"); preview.className = "relation-attachment-preview";
      if (isImageAttachment(item)) { preview.src = attachmentURL(nodeID, item.id, true); preview.alt = ""; preview.loading = "lazy"; } else { preview.textContent = "DOC"; }
      const info = document.createElement("span"), name = document.createElement("b"), details = document.createElement("small");
      name.textContent = item.filename; name.title = item.filename;
      details.textContent = `${formatBytes(item.size)} / SHA ${item.sha256.slice(0, 10)}`; info.append(name, details);
      const download = document.createElement("a"); download.href = `/api/relationships/nodes/${encodeURIComponent(nodeID)}/attachments/${encodeURIComponent(item.id)}`; download.textContent = "↓"; download.title = `Download ${item.filename}`; download.setAttribute("aria-label", `Download ${item.filename}`);
      const cover = document.createElement("button"); cover.type = "button"; cover.className = "relation-cover-action";
      const activeCover = node?.coverAttachmentId === item.id; cover.textContent = activeCover ? "★" : "☆"; cover.disabled = activeCover || !isImageAttachment(item); cover.hidden = !isImageAttachment(item); cover.title = I18n.t(activeCover ? "relations.activeCover" : "relations.setCover", { filename: item.filename }); cover.setAttribute("aria-label", cover.title); if (!activeCover) cover.addEventListener("click", () => setCover(nodeID, item));
      const remove = document.createElement("button"); remove.type = "button"; remove.textContent = "×"; remove.title = `Remove ${item.filename}`; remove.setAttribute("aria-label", `Remove ${item.filename}`); remove.addEventListener("click", () => removeAttachment(nodeID, item));
      row.append(preview, info, cover, download, remove); return row;
    }));
    if (animate) AmberMotion.revealList(el["relation-attachment-list"].children, { limit: 6, axis: "x" });
  }

  async function uploadAttachments(event) {
    const nodeID = el["relation-node-id"].value, files = [...event.target.files]; event.target.value = "";
    if (!nodeID || !files.length) return;
    const config = AmberAPI.getConfig().attachments;
    const existing = state.attachments.get(nodeID)?.length || 0;
    if (files.some((file) => file.size <= 0 || file.size > config.maxBytes)) { boardMessage(`BOARD ERROR / FILE EXCEEDS ${formatBytes(config.maxBytes)}`); return; }
    if (existing + files.length > config.maxPerNode) { boardMessage(`BOARD ERROR / MAXIMUM ${config.maxPerNode} FILES PER CARD`); return; }
    const caseData = await api("/api/case");
    if (!caseData) return;
    el["relation-attachment-add"].disabled = true;
    for (let index = 0; index < files.length; index += 1) {
      AmberMotion.typeText(el["relation-attachment-status"], `UPLOADING ${index + 1} / ${files.length}`);
      const body = new FormData(); body.append("caseId", caseData.id); body.append("file", files[index], files[index].name);
      const created = await api(`/api/relationships/nodes/${encodeURIComponent(nodeID)}/attachments`, { method: "POST", body });
      if (!created) break;
    }
    await syncRelationshipNode(nodeID);
    await loadAttachments(nodeID, true);
    notifyRelationshipChange(nodeID);
  }

  async function removeAttachment(nodeID, item) {
    if (!window.confirm(`${I18n.t("relations.removeAttachmentConfirm")}\n\n${item.filename}`)) return;
    const caseData = await api("/api/case"); if (!caseData) return;
    const removed = await api(`/api/relationships/nodes/${encodeURIComponent(nodeID)}/attachments/${encodeURIComponent(item.id)}`, { method: "DELETE", body: JSON.stringify({ caseId: caseData.id }) }, true);
    if (removed) { await syncRelationshipNode(nodeID); await loadAttachments(nodeID); notifyRelationshipChange(nodeID); }
  }

  async function setCover(nodeID, item) {
    const caseData = await api("/api/case"); if (!caseData) return;
    const updated = await api(`/api/relationships/nodes/${encodeURIComponent(nodeID)}/cover`, { method: "PUT", body: JSON.stringify({ caseId: caseData.id, attachmentId: item.id }) });
    if (!updated) return;
    replaceRelationshipNode(updated); renderCoverCards(); renderAttachmentList(nodeID, state.attachments.get(nodeID) || []); notifyRelationshipChange(nodeID);
  }

  async function syncRelationshipNode(nodeID) {
    const snapshot = await api("/api/relationships"); if (!snapshot) return;
    const node = snapshot.nodes.find((entry) => entry.id === nodeID); if (node) replaceRelationshipNode(node);
    renderCoverCards();
  }

  function replaceRelationshipNode(node) {
    const index = state.snapshot.nodes.findIndex((entry) => entry.id === node.id); if (index >= 0) state.snapshot.nodes[index] = node;
    const cyNode = state.cy?.getElementById(node.id); if (!cyNode?.length) return;
    cyNode.data({ ...cyNode.data(), ...node, cardLabel: `[${node.type.toUpperCase()}]\n${node.title}${node.subtitle ? `\n${node.subtitle}` : ""}` }); cyNode.toggleClass("has-cover", Boolean(node.coverAttachmentId));
  }

  function updateAttachmentCount(nodeID, count) {
    const node = state.snapshot.nodes.find((item) => item.id === nodeID); if (node) node.attachmentCount = count;
    const cyNode = state.cy?.getElementById(nodeID); if (cyNode?.length) cyNode.data("attachmentCount", count);
    renderAttachmentStacks();
  }

  function notifyRelationshipChange(nodeID) {
    const node = state.snapshot.nodes.find((item) => item.id === nodeID);
    document.dispatchEvent(new CustomEvent("amber:relationships-changed", { detail: { node, source: "relationships" } }));
  }

  function renderAttachmentStacks() {
    state.attachmentLayer?.remove();
    const layer = document.createElement("div"); layer.className = "relationship-attachment-layer"; state.attachmentLayer = layer;
    state.snapshot.nodes.filter((node) => node.attachmentCount > 0).forEach((node) => {
      const stack = document.createElement("span"); stack.className = "relationship-attachment-stack"; stack.dataset.nodeId = node.id; stack.setAttribute("role", "img"); stack.setAttribute("aria-label", I18n.t("relations.attachmentStack", { count: node.attachmentCount, title: node.title }));
      const count = document.createElement("b"); count.textContent = node.attachmentCount; stack.append(count); layer.append(stack);
    });
    el["relations-board"].append(layer); syncAttachmentStacks();
  }

  function renderCoverCards() {
    state.coverLayer?.remove();
    const layer = document.createElement("div"); layer.className = "relationship-cover-layer"; state.coverLayer = layer;
    state.snapshot.nodes.filter((node) => node.coverAttachmentId).forEach((node) => {
      const cover = document.createElement("div"); cover.className = "relationship-cover-card"; cover.dataset.nodeId = node.id; cover.setAttribute("role", "img"); cover.setAttribute("aria-label", I18n.t("relations.coverAlt", { title: node.title }));
      const photo = document.createElement("img"); photo.src = attachmentURL(node.id, node.coverAttachmentId, true); photo.alt = ""; photo.draggable = false;
      photo.addEventListener("error", () => { cover.hidden = true; state.cy?.getElementById(node.id).removeClass("has-cover"); });
      const text = document.createElement("span"), type = document.createElement("small"), title = document.createElement("b"), subtitle = document.createElement("em"); type.textContent = `[${node.type.toUpperCase()}]`; title.textContent = node.title; subtitle.textContent = node.subtitle || ""; text.append(type, title, subtitle); cover.append(photo, text); layer.append(cover);
    });
    el["relations-board"].append(layer); syncCoverCards();
  }

  function syncAttachmentStacks() {
    if (!state.cy || !state.attachmentLayer) return;
    state.attachmentLayer.querySelectorAll(".relationship-attachment-stack").forEach((stack) => {
      const node = state.cy.getElementById(stack.dataset.nodeId); if (!node.length) return;
      const position = node.renderedPosition(); stack.style.transform = `translate(${Math.round(position.x + node.renderedWidth() / 2 - 38)}px, ${Math.round(position.y + node.renderedHeight() / 2 - 28)}px)`;
    });
  }

  function syncCoverCards() {
    if (!state.cy || !state.coverLayer) return;
    state.coverLayer.querySelectorAll(".relationship-cover-card").forEach((cover) => {
      const node = state.cy.getElementById(cover.dataset.nodeId); if (!node.length) return;
      const position = node.renderedPosition(), inset = Math.max(2, Math.min(4, state.cy.zoom() * 3)), width = Math.max(1, node.renderedWidth() - inset * 2), height = Math.max(1, node.renderedHeight() - inset * 2);
      cover.style.transform = `translate(${Math.round(position.x - node.renderedWidth() / 2 + inset)}px, ${Math.round(position.y - node.renderedHeight() / 2 + inset)}px)`; cover.style.width = `${Math.round(width)}px`; cover.style.height = `${Math.round(height)}px`; cover.style.setProperty("--cover-font", `${Math.max(6, Math.min(12, width / 17))}px`); cover.classList.toggle("compact", height < 70);
    });
  }

  function syncBoardOverlays() { syncCoverCards(); syncAttachmentStacks(); }

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

  function resetSherlockConsole(node) {
    state.sherlock.source = node;
    state.sherlock.scan = null;
    state.sherlock.selected.clear();
    el["sherlock-launch"].hidden = true;
    el["sherlock-console"].hidden = true;
    el["sherlock-open"].textContent = I18n.t("sherlock.scan");
    el["sherlock-runtime"].textContent = state.sherlock.status?.ready ? `READY / ${state.sherlock.status.version}` : "RUNTIME NOT CHECKED";
    el["sherlock-username"].value = usernameFromNode(node);
    el["sherlock-results"].replaceChildren();
  }

  async function openSherlockConsole(prefill = "") {
    const source = state.snapshot.nodes.find((node) => node.id === state.selectedNodeID) || state.snapshot.nodes.find((node) => node.primary);
    if (!source) { boardMessage("SHERLOCK / SELECT A SOURCE CARD"); return; }
    if (state.selectedNodeID !== source.id) { state.cy?.getElementById(source.id).select(); showNode(source); }
    if (!state.sherlock.status) {
      el["sherlock-runtime"].textContent = "CHECKING RUNTIME";
      try {
        state.sherlock.status = await AmberAPI.requestJSON("/api/tools/sherlock/status");
      } catch { state.sherlock.status = { ready: false, message: "Sherlock status is unavailable" }; }
    }
    el["sherlock-runtime"].textContent = state.sherlock.status.ready ? `READY / ${state.sherlock.status.version}` : "UNAVAILABLE";
    el["sherlock-message"].textContent = state.sherlock.status.ready ? I18n.t("sherlock.hint") : (state.sherlock.status.message || I18n.t("sherlock.hint"));
    if (prefill) el["sherlock-username"].value = prefill;
    el["sherlock-open"].textContent = I18n.t("common.close");
    el["sherlock-launch"].hidden = Boolean(state.sherlock.scan);
    el["sherlock-console"].hidden = !state.sherlock.scan;
    AmberMotion.reveal(state.sherlock.scan ? el["sherlock-console"] : el["sherlock-launch"], { axis: "x", duration: 160 });
    if (!state.sherlock.scan) el["sherlock-username"].focus();
  }

  function toggleSherlockPanel() {
    if (!el["sherlock-launch"].hidden || !el["sherlock-console"].hidden) {
      el["sherlock-launch"].hidden = true; el["sherlock-console"].hidden = true; el["sherlock-open"].textContent = I18n.t("sherlock.scan"); return;
    }
    openSherlockConsole("");
  }

  function prepareSherlockScan() {
    const username = el["sherlock-username"].value.trim();
    if (!/^[A-Za-z0-9._-]{1,100}$/.test(username)) { AmberMotion.typeText(el["sherlock-message"], "Use 1-100 letters, digits, dots, underscores, or hyphens.", { tone: "error" }); return; }
    if (!state.sherlock.status?.ready) { AmberMotion.typeText(el["sherlock-message"], state.sherlock.status?.message || "Sherlock runtime is unavailable.", { tone: "error" }); return; }
    el["sherlock-confirm-username"].textContent = username;
    el["sherlock-confirm-runtime"].textContent = I18n.t("sherlock.externalRuntime", { version: state.sherlock.status.version });
    el["sherlock-confirm"].showModal();
  }

  async function startSherlockScan() {
    const source = state.sherlock.source, username = el["sherlock-username"].value.trim();
    if (!source) return;
    el["sherlock-confirm-start"].disabled = true;
    try {
      const caseData = await fetchJSON("/api/case");
      const scan = await fetchJSON("/api/tools/sherlock/scans", { method: "POST", body: JSON.stringify({ caseId: caseData.id, sourceNodeId: source.id, username, externalTrafficConfirmed: true }) });
      state.sherlock.selected.clear();
      el["sherlock-confirm"].close(); el["sherlock-launch"].hidden = true; el["sherlock-console"].hidden = false;
      updateSherlockScan(scan); AmberMotion.reveal(el["sherlock-console"], { axis: "x", duration: 200 }); connectSherlockStream(scan.id);
    } catch (error) { AmberMotion.typeText(el["sherlock-message"], `SCAN ERROR / ${error.message}`, { tone: "error" }); el["sherlock-confirm"].close(); }
    finally { el["sherlock-confirm-start"].disabled = false; }
  }

  function connectSherlockStream(scanID) {
    state.sherlock.stream?.close();
    const stream = new EventSource(`/api/tools/sherlock/scans/${encodeURIComponent(scanID)}/events`); state.sherlock.stream = stream;
    stream.addEventListener("snapshot", (event) => {
      const scan = JSON.parse(event.data); updateSherlockScan(scan);
      if (["completed", "failed", "cancelled"].includes(scan.state)) stream.close();
    });
    ["progress", "log", "completed", "failed", "cancelled"].forEach((type) => stream.addEventListener(type, (event) => {
      const payload = JSON.parse(event.data); if (payload.message) { if (["completed", "failed", "cancelled"].includes(type)) AmberMotion.typeText(el["sherlock-log"], payload.message, { tone: type === "failed" ? "error" : "ok" }); else el["sherlock-log"].textContent = payload.message; }
      if (payload.checked !== undefined) { el["sherlock-checked"].textContent = payload.checked; el["sherlock-claimed"].textContent = payload.claimed || 0; el["sherlock-errors"].textContent = payload.errors || 0; }
    }));
    stream.onerror = () => { if (["queued", "running"].includes(state.sherlock.scan?.state)) AmberMotion.typeText(el["sherlock-log"], "STREAM INTERRUPTED / RECONNECTING", { tone: "error" }); };
  }

  function updateSherlockScan(scan) {
    const previousState = state.sherlock.scan?.state;
    state.sherlock.scan = scan;
    el["sherlock-checked"].textContent = scan.checked || 0; el["sherlock-claimed"].textContent = scan.claimed || 0; el["sherlock-errors"].textContent = scan.errors || 0;
    el["sherlock-state"].textContent = scan.state.toUpperCase();
    const message = scan.error || (scan.state === "completed" ? `${scan.claimed} CANDIDATES / MANUAL REVIEW REQUIRED` : `SCANNING @${scan.username}`);
    if (previousState !== scan.state) AmberMotion.typeText(el["sherlock-log"], message, { tone: scan.error ? "error" : "ok" }); else el["sherlock-log"].textContent = message;
    el["sherlock-cancel"].disabled = !["queued", "running"].includes(scan.state);
    renderSherlockResults();
    if (previousState !== "completed" && scan.state === "completed") AmberMotion.revealList(el["sherlock-results"].children, { limit: 6, axis: "x" });
  }

  function renderSherlockResults() {
    const scan = state.sherlock.scan; if (!scan) return;
    const filter = el["sherlock-filter"].value, query = el["sherlock-search"].value.trim().toLowerCase();
    const visible = (scan.results || []).filter((result) => {
      if (filter === "claimed" && result.status !== "claimed") return false;
      if (filter === "problems" && ["claimed", "available"].includes(result.status)) return false;
      return !query || `${result.site} ${result.profileUrl}`.toLowerCase().includes(query);
    });
    el["sherlock-results"].replaceChildren(...visible.map((result) => {
      const row = document.createElement("div"); row.className = `sherlock-result${["claimed", "available"].includes(result.status) ? "" : " problem"}`;
      const checkbox = document.createElement("input"); checkbox.type = "checkbox"; checkbox.dataset.resultId = result.id; checkbox.disabled = result.status !== "claimed"; checkbox.checked = state.sherlock.selected.has(result.id);
      const copy = document.createElement("span"), name = document.createElement("b"), link = document.createElement(result.profileUrl ? "a" : "small"), status = document.createElement("em");
      name.textContent = result.site; link.textContent = result.profileUrl || "NO PROFILE URL"; link.title = result.profileUrl || ""; if (result.profileUrl) { link.href = result.profileUrl; link.target = "_blank"; link.rel = "noreferrer noopener"; } status.textContent = result.status.toUpperCase(); copy.append(name, link); row.append(checkbox, copy, status); return row;
    }));
    updateSherlockImportButton();
  }

  function toggleSherlockResult(event) {
    const input = event.target.closest("input[data-result-id]"); if (!input) return;
    if (input.checked) state.sherlock.selected.add(input.dataset.resultId); else state.sherlock.selected.delete(input.dataset.resultId);
    updateSherlockImportButton();
  }

  function updateSherlockImportButton() { el["sherlock-import"].disabled = state.sherlock.scan?.state !== "completed" || state.sherlock.selected.size === 0; el["sherlock-import"].textContent = state.sherlock.selected.size ? `${I18n.t("sherlock.addSelected")} / ${state.sherlock.selected.size}` : I18n.t("sherlock.addSelected"); }

  async function cancelSherlockScan() {
    if (!state.sherlock.scan) return;
    try { await fetchJSON(`/api/tools/sherlock/scans/${encodeURIComponent(state.sherlock.scan.id)}`, { method: "DELETE", body: "{}" }); AmberMotion.typeText(el["sherlock-log"], "CANCELLATION REQUESTED"); }
    catch (error) { AmberMotion.typeText(el["sherlock-log"], `CANCEL ERROR / ${error.message}`, { tone: "error" }); }
  }

  async function importSherlockResults() {
    const scan = state.sherlock.scan; if (!scan || !state.sherlock.selected.size) return;
    el["sherlock-import"].disabled = true; el["sherlock-log"].textContent = "WRITING TO DOSSIER / OBSIDIAN";
    try {
      const caseData = await fetchJSON("/api/case");
      const imported = await fetchJSON(`/api/tools/sherlock/scans/${encodeURIComponent(scan.id)}/import`, { method: "POST", body: JSON.stringify({ caseId: caseData.id, resultIds: [...state.sherlock.selected] }) });
      AmberMotion.typeText(el["sherlock-log"], `IMPORTED ${imported.selected} PROFILES / REPORT ATTACHED`);
      state.sherlock.selected.clear(); await load(); document.dispatchEvent(new CustomEvent("amber:timeline-changed", { detail: imported }));
    } catch (error) { AmberMotion.typeText(el["sherlock-log"], `IMPORT ERROR / ${error.message}`, { tone: "error" }); updateSherlockImportButton(); }
  }

  async function fetchJSON(url, options = {}) {
    return AmberAPI.requestJSON(url, options);
  }

  function usernameFromNode(node) { const value = (node.title || "").replace(/^@/, "").split(/\s|\//)[0]; return /^[A-Za-z0-9._-]{1,100}$/.test(value) ? value : ""; }

  async function api(url, options, noContent = false) {
    try { const payload = await AmberAPI.requestJSON(url, options); return noContent ? true : payload; }
    catch (error) { boardMessage(`BOARD ERROR / ${error.message}`); return null; }
  }

  function nodeTitle(id) { return state.snapshot.nodes.find((node) => node.id === id)?.title || id; }
  function csv(value) { return value.split(",").map((item) => item.trim()).filter(Boolean); }
  function clamp(value) { return Math.max(0, Math.min(1, value)); }
  function formatBytes(value) { if (value < 1024) return `${value} B`; if (value < 1048576) return `${(value / 1024).toFixed(1)} KiB`; return `${(value / 1048576).toFixed(1)} MiB`; }
  function isImageAttachment(item) { return AmberAPI.isImageMediaType(item.mediaType); }
  function attachmentURL(nodeID, attachmentID, inline = false) { return `/api/relationships/nodes/${encodeURIComponent(nodeID)}/attachments/${encodeURIComponent(attachmentID)}${inline ? "?inline=1" : ""}`; }
  document.addEventListener("amber:case-switched", refresh);
  return { init, refresh, openSherlock: openSherlockConsole, get loaded() { return state.loaded; } };
})();
