"use strict";

window.DossierWizard = (() => {
  let step = 1;
  let initialized = false;
  const el = {};

  function init() {
    if (initialized) return;
    ["case-wizard", "case-wizard-form", "case-wizard-close", "wizard-back", "wizard-next", "wizard-submit", "wizard-message", "wizard-confidence", "wizard-confidence-value", "wizard-add-identifier", "wizard-add-relation", "wizard-identifiers", "wizard-relations"].forEach((id) => { el[id] = document.getElementById(id); });
    el["case-wizard-close"].addEventListener("click", close);
    el["wizard-back"].addEventListener("click", () => showStep(step - 1));
    el["wizard-next"].addEventListener("click", next);
    el["wizard-confidence"].addEventListener("input", () => { el["wizard-confidence-value"].textContent = `${el["wizard-confidence"].value}%`; });
    el["wizard-add-identifier"].addEventListener("click", () => addIdentifier());
    el["wizard-add-relation"].addEventListener("click", () => addRelation());
    el["case-wizard-form"].addEventListener("submit", submit);
    el["case-wizard-form"].addEventListener("click", (event) => {
      const remove = event.target.closest("button[data-remove-row]");
      if (remove) remove.closest(".wizard-repeat-row").remove();
    });
    initialized = true;
  }

  function open() {
    init();
    showStep(1);
    el["wizard-message"].textContent = "";
    document.getElementById("wizard-sync-hint").textContent = document.getElementById("obsidian-open").dataset.state === "connected" ? "LOCAL + OBSIDIAN / READY" : "LOCAL MEMORY / OBSIDIAN OPTIONAL";
    el["case-wizard"].showModal();
    window.setTimeout(() => document.getElementById("wizard-case-name").focus(), 0);
  }

  function close() { el["case-wizard"].close(); }

  function showStep(nextStep) {
    step = Math.max(1, Math.min(3, nextStep));
    document.querySelectorAll("[data-wizard-step]").forEach((section) => { section.hidden = Number(section.dataset.wizardStep) !== step; });
    document.querySelectorAll("[data-step-target]").forEach((button) => { button.classList.toggle("active", Number(button.dataset.stepTarget) === step); button.classList.toggle("complete", Number(button.dataset.stepTarget) < step); });
    el["wizard-back"].hidden = step === 1;
    el["wizard-next"].hidden = step === 3;
    el["wizard-submit"].hidden = step !== 3;
    AmberMotion.reveal(document.querySelector(`[data-wizard-step="${step}"]`), { axis: "x", duration: 160 });
  }

  function next() {
    const section = document.querySelector(`[data-wizard-step="${step}"]`);
    const invalid = [...section.querySelectorAll("input,select,textarea")].find((field) => !field.checkValidity());
    if (invalid) { invalid.reportValidity(); return; }
    showStep(step + 1);
  }

  function addIdentifier(value = {}) {
    const row = document.createElement("div");
    row.className = "wizard-repeat-row identifier-row";
    row.innerHTML = '<select aria-label="Identifier type"><option value="username">USERNAME</option><option value="email">EMAIL</option><option value="phone">PHONE</option><option value="domain">DOMAIN</option><option value="ip">IP</option><option value="account">ACCOUNT</option><option value="other">OTHER</option></select><input aria-label="Identifier value" maxlength="240" required><button type="button" data-remove-row aria-label="Remove">X</button>';
    row.querySelector("select").value = value.type || "username";
    row.querySelector("input").value = value.value || "";
    el["wizard-identifiers"].append(row);
    AmberMotion.markGenerated(row);
    row.querySelector("input").focus();
  }

  function addRelation(value = {}) {
    const row = document.createElement("div");
    row.className = "wizard-repeat-row relation-row";
    row.innerHTML = '<select aria-label="Entity type"><option value="subject">SUBJECT</option><option value="organization">ORGANIZATION</option><option value="account">ACCOUNT</option><option value="location">LOCATION</option></select><input aria-label="Related entity name" maxlength="160" required><select aria-label="Risk"><option value="low">LOW</option><option value="medium">MEDIUM</option><option value="high">HIGH</option></select><button type="button" data-remove-row aria-label="Remove">X</button>';
    const selects = row.querySelectorAll("select");
    selects[0].value = value.type || "subject";
    selects[1].value = value.risk || "low";
    row.querySelector("input").value = value.name || "";
    el["wizard-relations"].append(row);
    AmberMotion.markGenerated(row);
    row.querySelector("input").focus();
  }

  async function submit(event) {
    event.preventDefault();
    const invalid = [...document.querySelectorAll('[data-wizard-step="3"] input, [data-wizard-step="3"] select')].find((field) => !field.checkValidity());
    if (invalid) { invalid.reportValidity(); return; }
    const occurred = document.getElementById("wizard-last-seen").value;
    const payload = {
      name: value("wizard-case-name"), owner: value("wizard-owner"), objective: value("wizard-objective"), tags: csv(value("wizard-tags")),
      subject: {
        codename: value("wizard-codename"), displayName: value("wizard-display-name"), risk: value("wizard-risk"), confidence: Number(value("wizard-confidence")), location: value("wizard-location"), lastSeen: occurred ? new Date(occurred).toISOString() : "", aliases: csv(value("wizard-aliases")),
        identifiers: [...el["wizard-identifiers"].querySelectorAll(".identifier-row")].map((row) => ({ type: row.querySelector("select").value, value: row.querySelector("input").value.trim() })),
        relations: [...el["wizard-relations"].querySelectorAll(".relation-row")].map((row) => { const selects = row.querySelectorAll("select"); return { type: selects[0].value, name: row.querySelector("input").value.trim(), risk: selects[1].value }; }),
      },
    };
    el["wizard-submit"].disabled = true;
    AmberMotion.typeText(el["wizard-message"], "CREATING DOSSIER...");
    try {
      const response = await fetch("/api/case", { method: "POST", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(payload) });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || `API ${response.status}`);
      AmberMotion.typeText(el["wizard-message"], result.sync.state === "synced" ? "DOSSIER CREATED / OBSIDIAN SYNCED" : result.sync.state === "sync_pending" ? "DOSSIER CREATED / SYNC PENDING" : "DOSSIER CREATED / LOCAL");
      document.dispatchEvent(new CustomEvent("amber:case-created", { detail: result }));
      window.setTimeout(() => { el["case-wizard"].close(); el["case-wizard-form"].reset(); el["wizard-identifiers"].replaceChildren(); el["wizard-relations"].replaceChildren(); el["wizard-confidence-value"].textContent = "40%"; }, 500);
    } catch (error) {
      AmberMotion.typeText(el["wizard-message"], `ERROR / ${error.message}`, { tone: "error" });
    } finally { el["wizard-submit"].disabled = false; }
  }

  function value(id) { return document.getElementById(id).value.trim(); }
  function csv(text) { return text.split(",").map((item) => item.trim()).filter(Boolean); }
  return { init, open };
})();
