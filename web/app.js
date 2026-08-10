"use strict";

const state = {
  caseData: null,
  selectedID: null,
  type: "all",
  status: "all",
  query: "",
  integrations: [],
  dossier: null,
  dossierDirty: false,
  connectorMessageKey: "connector.checking",
  connectorMessageError: false,
  timelineBackend: "memory",
  workView: "timeline-view",
  cases: [],
  casesBackend: "memory",
  avatar: { primaryNodeID: "", coverAttachmentID: "", requestID: 0 },
  panels: { dossierCollapsed: false, evidenceCollapsed: false },
};

const commands = [
  { name: "HELP", descriptionKey: "command.help", run: () => openPalette() },
  { name: "VERIFY", descriptionKey: "command.verify", run: () => setSelectedStatus("verified") },
  { name: "PENDING", descriptionKey: "command.pending", run: () => setSelectedStatus("pending") },
  { name: "FILTER ID", descriptionKey: "command.filterId", run: () => setTypeFilter("identity") },
  { name: "FILTER NET", descriptionKey: "command.filterNet", run: () => setTypeFilter("infrastructure") },
  { name: "FILTER MEDIA", descriptionKey: "command.filterMedia", run: () => setTypeFilter("media") },
  { name: "FILTER ALL", descriptionKey: "command.filterAll", run: () => setTypeFilter("all") },
  { name: "CLEAR", descriptionKey: "command.clear", run: clearFilters },
  { name: "SHERLOCK", descriptionKey: "command.sherlock", run: () => openSherlockCommand("") },
];

const elements = {};

document.addEventListener("DOMContentLoaded", init);

async function init() {
  cacheElements();
  const parameters = new URLSearchParams(window.location.search);
  const requestedLanguage = parameters.get("lang");
  I18n.setLanguage(["en", "ru"].includes(requestedLanguage) ? requestedLanguage : I18n.detect());
  elements["language-select"].value = I18n.language;
  bindEvents();
  restorePanelPreferences();
  const requestedView = parameters.get("view");
  if (["dossier", "timeline", "evidence"].includes(requestedView)) setMobileView(requestedView);
  drawAvatar();
  updateClock();
  window.setInterval(updateClock, 1000);
  await Promise.all([loadCase(), loadIntegrations()]);
  await loadCases(true);
  await window.AmberChecklist.init();
  window.setInterval(() => {
    if (!document.hidden && state.caseData && state.timelineBackend === "obsidian") loadTimeline(true);
  }, 15000);
  if (parameters.get("connector") === "obsidian") await openDossierEditor();
}

function cacheElements() {
  [
    "case-name", "case-id", "case-picker-open", "case-mobile-open", "api-state", "clock", "search", "status-filter", "type-filters", "create-dossier", "delete-dossier",
    "timeline-list", "empty-state", "visible-count", "verified-count", "evidence-content",
    "event-id", "command-input", "command-output", "open-palette", "command-palette",
    "palette-input", "command-list", "subject-codename", "subject-name", "subject-risk",
    "confidence", "confidence-meter", "last-seen", "location", "aliases", "alias-count",
    "identifiers", "identifier-count", "relations", "relation-count", "subject-avatar", "subject-avatar-image", "subject-avatar-upload", "subject-avatar-input", "subject-avatar-status",
    "language-select", "obsidian-open", "dossier-dialog", "dossier-close", "dossier-path",
    "dossier-sync-state", "connector-message", "dossier-refresh", "dossier-save",
    "dossier-content", "dossier-stats",
    "timeline-title", "timeline-backend", "timeline-add", "event-dialog", "event-form",
    "event-close", "event-cancel", "event-title", "event-type", "event-time", "event-summary",
    "event-source", "event-confidence", "case-picker", "case-picker-close", "case-picker-search",
    "case-picker-list", "case-picker-status", "case-picker-create", "delete-case-dialog", "toggle-dossier-panel", "toggle-evidence-panel",
    "delete-case-form", "delete-case-close", "delete-case-cancel", "delete-case-confirm", "delete-case-submit",
  ].forEach((id) => { elements[id] = document.getElementById(id); });
  elements.shell = document.querySelector(".app-shell");
  elements.evidenceTemplate = document.getElementById("evidence-template");
}

function bindEvents() {
  window.DossierWizard.init();
  elements["create-dossier"].addEventListener("click", window.DossierWizard.open);
  elements["case-picker-open"].addEventListener("click", openCasePicker);
  elements["case-mobile-open"].addEventListener("click", openCasePicker);
  elements["case-picker-close"].addEventListener("click", closeCasePicker);
  elements["case-picker-search"].addEventListener("input", renderCasePicker);
  elements["case-picker-list"].addEventListener("click", handleCasePickerSelection);
  elements["case-picker-create"].addEventListener("click", () => {
    if (state.dossierDirty) { elements["case-picker-status"].textContent = I18n.t("case.switchDirty"); return; }
    closeCasePicker(); window.DossierWizard.open();
  });
  elements["case-picker"].addEventListener("close", () => elements["case-picker-open"].setAttribute("aria-expanded", "false"));
  elements["delete-dossier"].addEventListener("click", openDeleteCaseDialog);
  elements["delete-case-close"].addEventListener("click", closeDeleteCaseDialog);
  elements["delete-case-cancel"].addEventListener("click", closeDeleteCaseDialog);
  elements["delete-case-confirm"].addEventListener("input", updateDeleteCaseConfirmation);
  elements["delete-case-form"].addEventListener("submit", deleteActiveCase);
  elements["toggle-dossier-panel"].addEventListener("click", () => togglePanel("dossier"));
  elements["toggle-evidence-panel"].addEventListener("click", () => togglePanel("evidence"));
  elements["language-select"].addEventListener("change", (event) => changeLanguage(event.target.value));
  elements["obsidian-open"].addEventListener("click", openDossierEditor);
  elements["dossier-close"].addEventListener("click", () => elements["dossier-dialog"].close());
  elements["dossier-refresh"].addEventListener("click", () => loadDossier(true));
  elements["dossier-save"].addEventListener("click", saveDossier);
  elements["dossier-content"].addEventListener("input", markDossierDirty);
  elements["subject-avatar-upload"].addEventListener("click", () => elements["subject-avatar-input"].click());
  elements["subject-avatar-input"].addEventListener("change", uploadSubjectAvatar);
  document.addEventListener("amber:relationships-changed", () => loadSubjectAvatar());
  document.querySelector(".workspace-tabs").addEventListener("click", switchWorkView);
  elements["timeline-add"].addEventListener("click", openEventDialog);
  elements["event-close"].addEventListener("click", () => elements["event-dialog"].close());
  elements["event-cancel"].addEventListener("click", () => elements["event-dialog"].close());
  elements["event-form"].addEventListener("submit", createTimelineEvent);
  document.addEventListener("amber:timeline-changed", (event) => loadTimeline(true, event.detail?.id || ""));
  document.addEventListener("amber:case-created", async (event) => {
    state.caseData = event.detail.case;
    state.selectedID = null;
    state.dossier = null;
    state.dossierDirty = false;
    elements["dossier-content"].value = "";
    state.timelineBackend = event.detail.sync.backend || "memory";
    elements["timeline-backend"].textContent = state.timelineBackend.toUpperCase();
    renderCase();
    await loadCases(true);
    refreshCaseModules();
    commandMessage(event.detail.sync.state === "sync_pending" ? "DOSSIER CREATED / SYNC PENDING" : "DOSSIER CREATED");
    if (elements["dossier-dialog"].open) loadDossier(false);
  });
  window.addEventListener("beforeunload", (event) => {
    if (!state.dossierDirty) return;
    event.preventDefault();
    event.returnValue = "";
  });
  elements["type-filters"].addEventListener("click", (event) => {
    const button = event.target.closest("button[data-filter]");
    if (button) setTypeFilter(button.dataset.filter);
  });
  elements["status-filter"].addEventListener("change", (event) => {
    state.status = event.target.value;
    renderTimeline();
  });
  elements.search.addEventListener("input", (event) => {
    state.query = event.target.value.trim().toLowerCase();
    renderTimeline();
  });
  elements["timeline-list"].addEventListener("click", (event) => {
    const remove = event.target.closest("button[data-delete-event]");
    if (remove) { deleteTimelineEvent(remove.dataset.deleteEvent); return; }
    const item = event.target.closest("button[data-event-id]");
    if (item) selectEvent(item.dataset.eventId);
  });
  document.querySelector(".mobile-tabs").addEventListener("click", (event) => {
    const button = event.target.closest("button[data-mobile-view]");
    if (!button) return;
    setMobileView(button.dataset.mobileView);
  });
  elements["command-input"].addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      runCommand(event.currentTarget.value);
      event.currentTarget.value = "";
    }
  });
  elements["open-palette"].addEventListener("click", openPalette);
  elements["palette-input"].addEventListener("input", renderCommands);
  elements["command-list"].addEventListener("click", (event) => {
    const button = event.target.closest("button[data-command]");
    if (!button) return;
    elements["command-palette"].close();
    runCommand(button.dataset.command);
  });
  document.addEventListener("keydown", (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      openPalette();
    } else if (event.key === "/" && !isTypingTarget(event.target)) {
      event.preventDefault();
      if (state.workView === "catalog-view") document.getElementById("catalog-search").focus();
      else if (state.workView === "relations-view") document.getElementById("relation-search").focus();
      else elements.search.focus();
    }
  });
}

async function loadCase() {
  try {
    const response = await fetch("/api/case", { headers: { Accept: "application/json" } });
    if (!response.ok) throw new Error(`API ${response.status}`);
    state.caseData = await response.json();
    await loadTimeline();
    state.selectedID = state.caseData.events[0]?.id || null;
    elements["api-state"].textContent = I18n.t("state.synced");
    enableControls();
    renderCase();
    AmberMotion.reveal(document.querySelector(".identity-block"), { axis: "x", duration: 200 });
    commandMessage("WORKSPACE READY");
  } catch (error) {
    elements["api-state"].textContent = I18n.t("state.offline");
    commandMessage(`CONNECTION ERROR / ${error.message}`, true);
    elements["evidence-content"].innerHTML = '<div class="loading-block"><b>API UNAVAILABLE</b><span></span><small>START WITH: go run .</small></div>';
  }
}

async function loadTimeline(silent = false, generatedID = "") {
  try {
    const snapshot = await request("/api/timeline", {});
    state.caseData.events = snapshot.events || [];
    state.timelineBackend = snapshot.backend || "memory";
    elements["timeline-backend"].textContent = state.timelineBackend.toUpperCase();
    if (state.selectedID && !state.caseData.events.some((event) => event.id === state.selectedID)) state.selectedID = state.caseData.events[0]?.id || null;
    if (silent) { renderTimeline(generatedID); renderEvidence(); }
  } catch (error) {
    if (!silent) throw error;
  }
}

async function loadCases(silent = false) {
  try {
    const snapshot = await request("/api/cases", {});
    state.cases = snapshot.cases || [];
    state.casesBackend = snapshot.backend || "memory";
    renderCasePicker();
  } catch (error) {
    if (!silent) commandMessage(error.message, true);
  }
}

async function openCasePicker() {
  await loadCases();
  elements["case-picker-search"].value = "";
  renderCasePicker();
  elements["case-picker"].showModal();
  elements["case-picker-open"].setAttribute("aria-expanded", "true");
  elements["case-picker-search"].focus();
}

function closeCasePicker() {
  if (elements["case-picker"].open) elements["case-picker"].close();
  elements["case-picker-open"].setAttribute("aria-expanded", "false");
}

function renderCasePicker() {
  if (!elements["case-picker-list"]) return;
  const query = elements["case-picker-search"].value.trim().toLowerCase();
  const items = state.cases.filter((item) => !query || `${item.id} ${item.name} ${item.subject}`.toLowerCase().includes(query));
  elements["case-picker-status"].textContent = `${state.casesBackend.toUpperCase()} / ${items.length}`;
  if (!items.length) {
    elements["case-picker-list"].replaceChildren(node("p", { class: "case-picker-empty" }, I18n.t("case.empty")));
    return;
  }
  elements["case-picker-list"].replaceChildren(...items.map((item) => {
    const button = node("button", { type: "button", class: `case-picker-item${item.active ? " active" : ""}`, "data-case-id": item.id });
    const copy = node("span");
    copy.append(node("strong", {}, item.name), node("small", {}, `${item.subject} / ${item.id}`));
    const updated = item.updatedAt ? new Intl.DateTimeFormat(I18n.language, { dateStyle: "short", timeStyle: "short" }).format(new Date(item.updatedAt)) : "--";
    button.append(copy, node("span", {}, item.active ? I18n.t("state.active") : updated));
    return button;
  }));
}

function handleCasePickerSelection(event) {
  const button = event.target.closest("button[data-case-id]");
  if (button) switchCase(button.dataset.caseId);
}

async function switchCase(caseID) {
  if (caseID === state.caseData.id) { closeCasePicker(); return; }
  if (state.dossierDirty) { commandMessage(I18n.t("case.switchDirty"), true); return; }
  try {
    const selected = await request("/api/cases/active", { method: "PUT", body: JSON.stringify({ caseId: caseID, expectedCaseId: state.caseData.id }) });
    await applyActiveCase(selected);
    closeCasePicker();
    commandMessage(`${selected.id} / ACTIVE`);
  } catch (error) { commandMessage(error.message, true); }
}

async function applyActiveCase(selected) {
  state.caseData = selected;
  state.selectedID = null;
  state.dossier = null;
  state.dossierDirty = false;
  elements["dossier-content"].value = "";
  await loadTimeline();
  state.selectedID = state.caseData.events[0]?.id || null;
  await loadCases(true);
  renderCase();
  AmberMotion.reveal(document.querySelector(".timeline"), { axis: "x", duration: 200 });
  refreshCaseModules();
}

function refreshCaseModules() {
  document.dispatchEvent(new CustomEvent("amber:case-switched", { detail: { case: state.caseData } }));
}

function openDeleteCaseDialog() {
  if (state.caseData.subject.codename === "UNASSIGNED") return;
  if (state.dossierDirty) { commandMessage(I18n.t("case.deleteDirty"), true); return; }
  elements["delete-case-form"].reset();
  elements["delete-case-confirm"].placeholder = state.caseData.id;
  updateDeleteCaseConfirmation();
  elements["delete-case-dialog"].showModal();
  elements["delete-case-confirm"].focus();
}

function closeDeleteCaseDialog() { elements["delete-case-dialog"].close(); }

function updateDeleteCaseConfirmation() {
  elements["delete-case-submit"].disabled = elements["delete-case-confirm"].value.trim() !== state.caseData.id;
}

async function deleteActiveCase(event) {
  event.preventDefault();
  const caseID = state.caseData.id;
  elements["delete-case-submit"].disabled = true;
  try {
    const result = await request(`/api/cases/${encodeURIComponent(caseID)}`, { method: "DELETE", body: JSON.stringify({ confirmCaseId: elements["delete-case-confirm"].value.trim(), expectedCaseId: caseID }) });
    state.cases = result.cases || [];
    state.casesBackend = result.backend || "memory";
    await applyActiveCase(result.case);
    closeDeleteCaseDialog();
    commandMessage(I18n.t("case.deleted"));
  } catch (error) {
    commandMessage(error.message, true);
    elements["delete-case-submit"].disabled = false;
  }
}

async function deleteTimelineEvent(eventID) {
  const event = state.caseData.events.find((item) => item.id === eventID);
  if (!event || !window.confirm(`${I18n.t("timeline.deleteConfirm")}\n\n${event.title}`)) return;
  try {
    await request(`/api/events/${encodeURIComponent(eventID)}`, { method: "DELETE", body: JSON.stringify({ caseId: state.caseData.id }) });
    state.caseData.events = state.caseData.events.filter((item) => item.id !== eventID);
    if (state.selectedID === eventID) state.selectedID = state.caseData.events[0]?.id || null;
    renderTimeline();
    renderEvidence();
    commandMessage(`${eventID} / MOVED TO TRASH`);
  } catch (error) { commandMessage(error.message, true); }
}

function openEventDialog() {
  elements["event-form"].reset();
  const now = new Date(Date.now() - new Date().getTimezoneOffset() * 60000);
  elements["event-time"].value = now.toISOString().slice(0, 16);
  elements["event-confidence"].value = 50;
  elements["event-dialog"].showModal();
  elements["event-title"].focus();
}

async function createTimelineEvent(event) {
  event.preventDefault();
  const occurredAt = new Date(elements["event-time"].value);
  const payload = {
    occurredAt: occurredAt.toISOString(),
    time: occurredAt.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false }),
    date: occurredAt.toLocaleDateString("en-GB", { day: "2-digit", month: "short" }).toUpperCase(),
    type: elements["event-type"].value,
    title: elements["event-title"].value.trim(),
    summary: elements["event-summary"].value.trim(),
    source: elements["event-source"].value.trim() || "ANALYST",
    sourceUrl: "",
    confidence: Number(elements["event-confidence"].value),
    status: "pending",
    fingerprint: "",
    indicators: [],
    notes: [],
  };
  try {
    const created = await request("/api/timeline/events", { method: "POST", body: JSON.stringify(payload) });
    state.caseData.events.unshift(created);
    state.selectedID = created.id;
    elements["event-dialog"].close();
    renderTimeline(created.id);
    renderEvidence();
    commandMessage(`${created.id} / EVENT ADDED`);
  } catch (error) { commandMessage(error.message, true); }
}

function switchWorkView(event) {
  const button = event.target.closest("button[data-work-view]");
  if (!button) return;
  const changed = state.workView !== button.dataset.workView;
  state.workView = button.dataset.workView;
  document.querySelectorAll("[data-work-view]").forEach((item) => item.classList.toggle("active", item === button));
  document.querySelectorAll(".work-view").forEach((view) => { view.hidden = view.id !== state.workView; });
  elements.shell.dataset.workspace = ["catalog-view", "relations-view"].includes(state.workView) ? "full" : "case";
  syncWorkspaceTitle();
  if (state.workView === "map-view") window.AmberMap.init();
  if (state.workView === "relations-view") window.AmberRelations.init();
  if (state.workView === "catalog-view") window.AmberCatalog.init();
  if (changed) AmberMotion.reveal(document.getElementById(state.workView), { axis: "x", duration: 160 });
}

function syncWorkspaceTitle() {
  const key = state.workView === "catalog-view" ? "catalog.title" : state.workView === "relations-view" ? "relations.title" : state.workView === "map-view" ? "map.title" : "timeline.title";
  elements["timeline-title"].textContent = I18n.t(key);
}

async function loadIntegrations() {
  try {
    state.integrations = await request("/api/integrations", {});
  } catch {
    state.integrations = [];
  }
  updateConnectorButton();
}

function obsidianIntegration() {
  return state.integrations.find((integration) => integration.metadata.id === "obsidian") || null;
}

function updateConnectorButton() {
  const integration = obsidianIntegration();
  const connectorState = integration?.status.state || "offline";
  elements["obsidian-open"].dataset.state = connectorState;
  elements["obsidian-open"].title = integration?.status.message || I18n.t("connector.unavailable");
}

async function openDossierEditor() {
  const integration = obsidianIntegration();
  elements["dossier-dialog"].showModal();
  if (!integration || integration.status.state !== "connected") {
    const unconfigured = integration?.status.state === "unconfigured";
    elements["dossier-content"].disabled = true;
    elements["dossier-refresh"].disabled = true;
    elements["dossier-save"].disabled = true;
    elements["dossier-path"].textContent = "OBSIDIAN_VAULT";
    setDossierState(unconfigured ? "unconfigured" : "offline");
    setConnectorMessage(unconfigured ? "connector.setup" : "connector.unavailable", true);
    return;
  }
  if (!state.dossierDirty) await loadDossier(false);
}

async function loadDossier(confirmDiscard) {
  if (confirmDiscard && state.dossierDirty && !window.confirm(I18n.t("connector.reloadConfirm"))) return;
  elements["dossier-content"].disabled = true;
  elements["dossier-refresh"].disabled = true;
  elements["dossier-save"].disabled = true;
  setDossierState("loading");
  setConnectorMessage("connector.checking");
  try {
    const dossier = await request("/api/integrations/obsidian/dossier", {});
    state.dossier = dossier;
    state.dossierDirty = !dossier.exists;
    elements["dossier-content"].value = dossier.content;
    elements["dossier-content"].disabled = false;
    elements["dossier-refresh"].disabled = false;
    elements["dossier-save"].disabled = false;
    elements["dossier-path"].textContent = dossier.path;
    setDossierState(dossier.exists ? "synced" : "modified");
    setConnectorMessage(dossier.exists ? "connector.loaded" : "connector.draft");
    updateDossierStats();
  } catch (error) {
    setDossierState("offline");
    setConnectorMessage("connector.unavailable", true);
    commandMessage(error.message, true);
  }
}

async function saveDossier() {
  if (!state.dossier) return;
  elements["dossier-save"].disabled = true;
  setDossierState("saving");
  try {
    const dossier = await request("/api/integrations/obsidian/dossier", {
      method: "PUT",
      body: JSON.stringify({
        caseId: state.dossier.caseId,
        content: elements["dossier-content"].value,
        expectedModifiedAt: state.dossier.modifiedAt || "",
      }),
    });
    state.dossier = dossier;
    state.dossierDirty = false;
    elements["dossier-path"].textContent = dossier.path;
    setDossierState("synced");
    setConnectorMessage("connector.saved");
    commandMessage(I18n.t("connector.saved"));
  } catch (error) {
    if (error.status === 409) {
      setDossierState("conflict");
      setConnectorMessage("connector.conflictMessage", true);
    } else {
      setDossierState("offline");
      setConnectorMessage("connector.unavailable", true);
    }
    commandMessage(error.message, true);
  } finally {
    elements["dossier-save"].disabled = false;
  }
}

function markDossierDirty() {
  state.dossierDirty = elements["dossier-content"].value !== (state.dossier?.content || "");
  setDossierState(state.dossierDirty ? "modified" : "synced");
  updateDossierStats();
}

function setDossierState(name) {
  elements["dossier-sync-state"].dataset.state = name;
  elements["dossier-sync-state"].textContent = I18n.t(`connector.${name}`);
}

function setConnectorMessage(key, isError = false) {
  state.connectorMessageKey = key;
  state.connectorMessageError = isError;
  elements["connector-message"].textContent = I18n.t(key);
  elements["connector-message"].classList.toggle("error", isError);
}

function updateDossierStats() {
  elements["dossier-stats"].textContent = I18n.t("connector.chars", { count: elements["dossier-content"].value.length });
}

function changeLanguage(language) {
  I18n.setLanguage(language);
  if (state.caseData) {
    elements["api-state"].textContent = I18n.t("state.synced");
    renderCase();
  }
  updateConnectorButton();
  setConnectorMessage(state.connectorMessageKey, state.connectorMessageError);
  const dossierState = elements["dossier-sync-state"].dataset.state;
  if (dossierState) setDossierState(dossierState);
  updateDossierStats();
  if (elements["command-palette"].open) renderCommands();
  if (elements["case-picker"].open) renderCasePicker();
  syncWorkspaceTitle();
  if (window.AmberCatalog?.loaded) {
    window.AmberCatalog.renderCategories();
    window.AmberCatalog.render();
  }
  window.AmberChecklist?.render();
  applyPanelPreferences();
}

function restorePanelPreferences() {
  try {
    const saved = JSON.parse(localStorage.getItem("amberdesk.panels") || "{}");
    state.panels.dossierCollapsed = saved.dossierCollapsed === true;
    state.panels.evidenceCollapsed = saved.evidenceCollapsed === true;
  } catch {
    state.panels = { dossierCollapsed: false, evidenceCollapsed: false };
  }
  applyPanelPreferences();
}

function togglePanel(name) {
  const key = `${name}Collapsed`;
  state.panels[key] = !state.panels[key];
  localStorage.setItem("amberdesk.panels", JSON.stringify(state.panels));
  applyPanelPreferences();
  if (!state.panels[key]) AmberMotion.reveal(document.getElementById(`${name}-panel`), { axis: "x", duration: 160 });
  window.dispatchEvent(new Event("resize"));
}

function applyPanelPreferences() {
  if (!elements.shell) return;
  elements.shell.dataset.dossierCollapsed = String(state.panels.dossierCollapsed);
  elements.shell.dataset.evidenceCollapsed = String(state.panels.evidenceCollapsed);
  const left = elements["toggle-dossier-panel"], right = elements["toggle-evidence-panel"];
  if (!left || !right) return;
  left.textContent = state.panels.dossierCollapsed ? ">" : "<";
  right.textContent = state.panels.evidenceCollapsed ? "<" : ">";
  left.setAttribute("aria-expanded", String(!state.panels.dossierCollapsed));
  right.setAttribute("aria-expanded", String(!state.panels.evidenceCollapsed));
  const leftLabel = I18n.t(state.panels.dossierCollapsed ? "panels.showDossier" : "panels.hideDossier");
  const rightLabel = I18n.t(state.panels.evidenceCollapsed ? "panels.showEvidence" : "panels.hideEvidence");
  left.title = leftLabel;
  right.title = rightLabel;
  left.setAttribute("aria-label", leftLabel);
  right.setAttribute("aria-label", rightLabel);
}

function enableControls() {
  [elements.search, elements["status-filter"], elements["command-input"], elements["open-palette"]]
    .forEach((control) => { control.disabled = false; });
}

function renderCase() {
  const data = state.caseData;
  const subject = data.subject;
  elements["case-name"].textContent = data.name;
  elements["case-id"].textContent = data.id;
  elements["create-dossier"].hidden = subject.codename !== "UNASSIGNED";
  elements["delete-dossier"].hidden = subject.codename === "UNASSIGNED";
  document.querySelector(".record-state").hidden = subject.codename === "UNASSIGNED";
  elements["subject-codename"].textContent = subject.codename;
  elements["subject-name"].textContent = subject.displayName;
  elements["subject-risk"].textContent = I18n.t(`risk.${subject.risk}`);
  elements.confidence.textContent = `${subject.confidence}%`;
  elements["confidence-meter"].style.width = `${subject.confidence}%`;
  elements["last-seen"].textContent = subject.lastSeen;
  elements.location.textContent = subject.location;
  elements.aliases.replaceChildren(...subject.aliases.map((alias) => node("li", {}, alias)));
  elements["alias-count"].textContent = pad(subject.aliases.length);
  elements.identifiers.replaceChildren(...subject.identifiers.map((item) => {
    const row = node("div");
    row.append(node("dt", {}, item.type), node("dd", {}, item.value));
    return row;
  }));
  elements["identifier-count"].textContent = pad(subject.identifiers.length);
  elements.relations.replaceChildren(...subject.relations.map((relation) => {
    const item = node("li");
    const copy = node("span");
    copy.append(node("b", {}, relation.name), node("small", {}, relation.type.toUpperCase()));
    item.append(node("i"), copy, node("em", { "data-risk": relation.risk }, I18n.t(`risk.${relation.risk}`)));
    return item;
  }));
  elements["relation-count"].textContent = pad(subject.relations.length);
  loadSubjectAvatar();
  renderTimeline();
  renderEvidence();
}

function filteredEvents() {
  if (!state.caseData) return [];
  return state.caseData.events.filter((event) => {
    const typeMatch = state.type === "all" || event.type === state.type;
    const statusMatch = state.status === "all" || event.status === state.status;
    const haystack = [event.id, event.title, event.summary, event.source, event.fingerprint, ...(event.indicators || [])].join(" ").toLowerCase();
    return typeMatch && statusMatch && (!state.query || haystack.includes(state.query));
  });
}

function renderTimeline(generatedID = "") {
  const events = filteredEvents();
  elements["visible-count"].textContent = pad(events.length);
  elements["verified-count"].textContent = pad(state.caseData.events.filter((event) => event.status === "verified").length);
  elements["empty-state"].hidden = events.length !== 0;
  elements["timeline-list"].hidden = events.length === 0;
  elements["timeline-list"].replaceChildren(...events.map((event) => {
    const row = node("div", { class: "timeline-row" });
    const button = node("button", { type: "button", class: `timeline-item${event.id === state.selectedID ? " active" : ""}`, "data-event-id": event.id });
    const time = node("time", {}, event.time);
    time.append(node("small", {}, event.date));
    const copy = node("span", { class: "event-copy" });
    copy.append(node("strong", {}, event.title), node("p", {}, event.summary), node("small", {}, `${(event.source || "ANALYST").toUpperCase()} / ${event.id}`));
    const score = node("span", { class: "event-score" });
    score.append(node("b", {}, `${event.confidence}%`), node("span", { class: `status-dot ${event.status}` }, I18n.t(`status.${event.status}`)));
    button.append(time, node("span", { class: "timeline-marker" }), copy, score);
    const remove = node("button", { type: "button", class: "timeline-delete", "data-delete-event": event.id, title: I18n.t("common.delete"), "aria-label": `${I18n.t("common.delete")}: ${event.title}` }, "DEL");
    row.append(button, remove);
    return row;
  }));
  if (generatedID) AmberMotion.markGenerated(elements["timeline-list"].querySelector(`[data-event-id="${CSS.escape(generatedID)}"]`)?.closest(".timeline-row"));
}

function renderEvidence() {
  const event = selectedEvent();
  if (!event) {
    elements["event-id"].textContent = "--";
    elements["evidence-content"].innerHTML = `<div class="loading-block"><b>${I18n.t("evidence.select")}</b></div>`;
    return;
  }
  elements["event-id"].textContent = event.id;
  const fragment = elements.evidenceTemplate.content.cloneNode(true);
  I18n.apply(fragment);
  fragment.querySelector(".type-code").textContent = `${event.type} / ${event.source}`;
  const badge = fragment.querySelector(".status-badge");
  badge.textContent = I18n.t(`status.${event.status}`);
  badge.classList.add(event.status);
  fragment.querySelector(".evidence-title").textContent = event.title;
  fragment.querySelector(".evidence-summary").textContent = event.summary;
  fragment.querySelector(".meta-source").textContent = event.sourceURL;
  fragment.querySelector(".meta-time").textContent = `${event.date} / ${event.time}`;
  fragment.querySelector(".meta-confidence").textContent = `${event.confidence}%`;
  fragment.querySelector(".evidence-meta .meter i").style.width = `${event.confidence}%`;
  fragment.querySelector(".meta-fingerprint").textContent = event.fingerprint;
  const indicators = event.indicators || [];
  const eventNotes = event.notes || [];
  fragment.querySelector(".indicator-list").replaceChildren(...indicators.map((value) => node("li", {}, value)));
  fragment.querySelector(".note-count").textContent = pad(eventNotes.length);
  const notes = fragment.querySelector(".notes-list");
  notes.replaceChildren(...(eventNotes.length ? eventNotes.map((note) => {
    const item = node("li", {}, note.text);
    item.append(node("time", {}, note.createdAt));
    return item;
  }) : [node("li", { class: "no-notes" }, I18n.t("evidence.noNotes"))]));
  const verifyButton = fragment.querySelector(".verify-button");
  const nextStatus = event.status === "verified" ? "pending" : "verified";
  verifyButton.dataset.next = nextStatus;
  verifyButton.textContent = I18n.t(nextStatus === "verified" ? "evidence.markVerified" : "evidence.returnPending");
  verifyButton.addEventListener("click", () => setSelectedStatus(nextStatus));
  fragment.querySelector(".copy-button").addEventListener("click", () => copyFingerprint(event.fingerprint));
  fragment.querySelector(".note-form").addEventListener("submit", addNote);
  elements["evidence-content"].replaceChildren(fragment);
}

function selectEvent(id) {
  state.selectedID = id;
  renderTimeline();
  renderEvidence();
  if (window.matchMedia("(max-width: 880px)").matches) setMobileView("evidence");
}

function selectedEvent() {
  return state.caseData?.events.find((event) => event.id === state.selectedID) || null;
}

async function setSelectedStatus(status) {
  const event = selectedEvent();
  if (!event) return commandMessage("NO EVENT SELECTED", true);
  try {
    const updated = await request(`/api/events/${encodeURIComponent(event.id)}/status`, {
      method: "PATCH",
      body: JSON.stringify({ status }),
    });
    replaceEvent(updated);
    renderTimeline();
    renderEvidence();
    AmberMotion.pulse(elements["timeline-list"].querySelector(`[data-event-id="${CSS.escape(event.id)}"] .status-dot`));
    AmberMotion.pulse(elements["evidence-content"].querySelector(".status-badge"));
    commandMessage(`${event.id} / STATUS ${status.toUpperCase()}`);
  } catch (error) {
    commandMessage(error.message, true);
  }
}

async function addNote(event) {
  event.preventDefault();
  const textarea = event.currentTarget.querySelector("textarea");
  const text = textarea.value.trim();
  if (!text) return;
  try {
    const updated = await request(`/api/events/${encodeURIComponent(state.selectedID)}/notes`, {
      method: "POST",
      body: JSON.stringify({ text }),
    });
    replaceEvent(updated);
    renderEvidence();
    AmberMotion.markGenerated(elements["evidence-content"].querySelector(".notes-list li:last-child"));
    commandMessage(`${updated.id} / NOTE ADDED`);
  } catch (error) {
    commandMessage(error.message, true);
  }
}

async function request(url, options) {
  const response = await fetch(url, { ...options, headers: { "Content-Type": "application/json", Accept: "application/json" } });
  const payload = await response.json();
  if (!response.ok) {
    const error = new Error(payload.error || `API ${response.status}`);
    error.status = response.status;
    throw error;
  }
  return payload;
}

function replaceEvent(updated) {
  const index = state.caseData.events.findIndex((event) => event.id === updated.id);
  if (index >= 0) state.caseData.events[index] = updated;
}

function setTypeFilter(type) {
  state.type = type;
  document.querySelectorAll("[data-filter]").forEach((button) => button.classList.toggle("active", button.dataset.filter === type));
  renderTimeline();
}

function clearFilters() {
  state.query = "";
  state.status = "all";
  elements.search.value = "";
  elements["status-filter"].value = "all";
  setTypeFilter("all");
  commandMessage("FILTERS CLEARED");
}

function setMobileView(view) {
  elements.shell.dataset.view = view;
  document.querySelectorAll("[data-mobile-view]").forEach((button) => button.classList.toggle("active", button.dataset.mobileView === view));
}

function runCommand(raw) {
  const original = raw.trim();
  const input = original.toUpperCase();
  if (!input) return;
  const sherlockMatch = original.match(/^SHERLOCK(?:\s+([A-Za-z0-9._-]{1,100}))?$/i);
  if (sherlockMatch) { openSherlockCommand(sherlockMatch[1] || ""); commandMessage(`SHERLOCK${sherlockMatch[1] ? ` @${sherlockMatch[1]}` : ""} / READY`); return; }
  const selectMatch = input.match(/^SELECT\s+(EV-\d+)$/);
  if (selectMatch) {
    const found = state.caseData.events.some((event) => event.id === selectMatch[1]);
    if (found) { selectEvent(selectMatch[1]); commandMessage(`${selectMatch[1]} SELECTED`); }
    else commandMessage("EVENT NOT FOUND", true);
    return;
  }
  const command = commands.find((item) => item.name === input);
  if (!command) return commandMessage(`UNKNOWN COMMAND / ${input}`, true);
  command.run();
  if (input !== "HELP") commandMessage(`${input} / OK`);
}

async function openSherlockCommand(username) {
  document.querySelector('[data-work-view="relations-view"]')?.click();
  await window.AmberRelations.init();
  await window.AmberRelations.openSherlock(username);
}

function openPalette() {
  elements["palette-input"].value = "";
  renderCommands();
  elements["command-palette"].showModal();
  window.setTimeout(() => elements["palette-input"].focus(), 0);
}

function renderCommands() {
  const query = elements["palette-input"].value.trim().toLowerCase();
  const visible = commands.filter((command) => `${command.name} ${I18n.t(command.descriptionKey)}`.toLowerCase().includes(query));
  elements["command-list"].replaceChildren(...visible.map((command) => {
    const button = node("button", { type: "button", class: "command-option", "data-command": command.name });
    button.append(node("b", {}, command.name), node("span", {}, I18n.t(command.descriptionKey)));
    return button;
  }));
}

async function copyFingerprint(value) {
  try {
    await navigator.clipboard.writeText(value);
    commandMessage("FINGERPRINT COPIED");
  } catch {
    commandMessage("CLIPBOARD UNAVAILABLE", true);
  }
}

function commandMessage(message, isError = false) {
  AmberMotion.typeText(elements["command-output"], message, { tone: isError ? "error" : "ok" });
}

function updateClock() {
  elements.clock.textContent = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }).format(new Date());
}

function drawAvatar() {
  const canvas = elements["subject-avatar"];
  const context = canvas.getContext("2d");
  context.fillStyle = "#0a0705";
  context.fillRect(0, 0, canvas.width, canvas.height);
  context.strokeStyle = "#6d4820";
  context.lineWidth = 2;
  for (let offset = 8; offset < canvas.width; offset += 16) {
    context.beginPath(); context.moveTo(offset, 0); context.lineTo(offset, canvas.height); context.stroke();
    context.beginPath(); context.moveTo(0, offset); context.lineTo(canvas.width, offset); context.stroke();
  }
  context.strokeStyle = "#f5b94e";
  context.strokeRect(29, 29, 54, 54);
  context.beginPath(); context.moveTo(56, 16); context.lineTo(56, 96); context.stroke();
  context.beginPath(); context.moveTo(16, 56); context.lineTo(96, 56); context.stroke();
}

async function loadSubjectAvatar() {
  const requestID = ++state.avatar.requestID;
  if (!state.caseData || state.caseData.subject.codename === "UNASSIGNED") {
    state.avatar.primaryNodeID = ""; state.avatar.coverAttachmentID = ""; setAvatarFallback(); elements["subject-avatar-upload"].disabled = true; return;
  }
  elements["subject-avatar-upload"].disabled = false;
  try {
    const snapshot = await request("/api/relationships", {});
    if (requestID !== state.avatar.requestID) return;
    const primary = (snapshot.nodes || []).find((item) => item.primary);
    state.avatar.primaryNodeID = primary?.id || "";
    state.avatar.coverAttachmentID = primary?.coverAttachmentId || "";
    if (!primary?.coverAttachmentId) { setAvatarFallback(); return; }
    const image = elements["subject-avatar-image"];
    image.onload = () => { if (requestID !== state.avatar.requestID) return; image.hidden = false; elements["subject-avatar"].hidden = true; };
    image.onerror = () => { if (requestID === state.avatar.requestID) setAvatarFallback(); };
    image.src = `/api/relationships/nodes/${encodeURIComponent(primary.id)}/attachments/${encodeURIComponent(primary.coverAttachmentId)}?inline=1`;
  } catch {
    if (requestID === state.avatar.requestID) setAvatarFallback();
  }
}

function setAvatarFallback() {
  const image = elements["subject-avatar-image"];
  image.onload = null; image.onerror = null; image.removeAttribute("src"); image.hidden = true;
  elements["subject-avatar"].hidden = false;
}

async function uploadSubjectAvatar(event) {
  const file = event.target.files[0]; event.target.value = "";
  if (!file) return;
  if (!["image/jpeg", "image/png", "image/webp", "image/gif"].includes(file.type)) { setAvatarStatus("avatar.invalidType", true); return; }
  if (file.size <= 0 || file.size > 10 * 1024 * 1024) { setAvatarStatus("avatar.invalidSize", true); return; }
  if (!state.avatar.primaryNodeID) await loadSubjectAvatar();
  const nodeID = state.avatar.primaryNodeID, caseID = state.caseData?.id;
  if (!nodeID || !caseID) { setAvatarStatus("avatar.noPrimary", true); return; }
  const shell = elements["subject-avatar"].parentElement;
  shell.classList.add("uploading"); elements["subject-avatar-upload"].disabled = true; setAvatarStatus("avatar.uploading");
  try {
    const body = new FormData(); body.append("caseId", caseID); body.append("file", file, file.name);
    const uploadResponse = await fetch(`/api/relationships/nodes/${encodeURIComponent(nodeID)}/attachments`, { method: "POST", headers: { Accept: "application/json" }, body });
    const attachment = await uploadResponse.json();
    if (!uploadResponse.ok) throw new Error(attachment.error || `API ${uploadResponse.status}`);
    const coverResponse = await fetch(`/api/relationships/nodes/${encodeURIComponent(nodeID)}/cover`, { method: "PUT", headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify({ caseId: caseID, attachmentId: attachment.id }) });
    const cover = await coverResponse.json();
    if (!coverResponse.ok) { setAvatarStatus("avatar.coverFailed", true); return; }
    state.avatar.coverAttachmentID = cover.coverAttachmentId;
    await loadSubjectAvatar();
    setAvatarStatus("avatar.saved");
    document.dispatchEvent(new CustomEvent("amber:relationships-changed", { detail: { node: cover, source: "dossier-avatar" } }));
  } catch (error) { elements["subject-avatar-status"].textContent = error.message; commandMessage(`AVATAR / ${error.message}`, true); }
  finally { shell.classList.remove("uploading"); elements["subject-avatar-upload"].disabled = false; }
}

function setAvatarStatus(key, isError = false) {
  const message = I18n.t(key); elements["subject-avatar-status"].textContent = message; commandMessage(message, isError);
}

function node(tag, attributes = {}, text = "") {
  const element = document.createElement(tag);
  Object.entries(attributes).forEach(([name, value]) => element.setAttribute(name, value));
  if (text) element.textContent = text;
  return element;
}

function pad(value) { return String(value).padStart(2, "0"); }
function isTypingTarget(target) { return ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName); }
