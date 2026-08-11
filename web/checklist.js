"use strict";

window.AmberChecklist = (() => {
  const state = { initialized: false, snapshot: null, caseID: "", openPhase: "" };
  const el = {};
  const phaseRU = { scope: "ЦЕЛЬ И ГРАНИЦЫ", seeds: "ИСХОДНЫЕ ДАННЫЕ", plan: "ПЛАН ПОИСКА", collect: "СБОР И СОХРАНЕНИЕ", verify: "ПРОВЕРКА", structure: "СВЯЗИ, ВРЕМЯ И ГЕОГРАФИЯ", analyze: "АНАЛИЗ И ГИПОТЕЗЫ", report: "ИТОГ И ПЕРЕСМОТР" };
  const taskRU = {
    "scope-question": "Запишите точный вопрос расследования", "scope-subject": "Определите объект, период и географию", "scope-known": "Отделите известные факты от предположений", "scope-ethics": "Зафиксируйте правовые, этические и безопасные границы", "scope-stop": "Определите критерии завершения и остановки",
    "seeds-identifiers": "Запишите все известные идентификаторы и их источники", "seeds-variants": "Добавьте псевдонимы и варианты написания", "seeds-photos": "Прикрепите исходные фотографии и файлы", "seeds-entities": "Добавьте известных людей и организации", "seeds-hypotheses": "Запишите начальные гипотезы как непроверенные",
    "plan-directions": "Выберите самые ценные направления поиска", "plan-queries": "Подготовьте запросы и языковые варианты", "plan-tools": "Выберите инструменты и изучите их ограничения", "plan-priority": "Сначала проверьте источники, которые могут исчезнуть", "plan-negative": "Фиксируйте проверки, которые не дали результата",
    "collect-original": "Сохраните оригинальный URL и отличите репосты", "collect-access": "Запишите автора и время доступа", "collect-file": "Прикрепите оригинальный файл или точную копию", "collect-context": "Сохраните окружающий текст и контекст", "collect-unaltered": "Храните неизмененную копию медиа", "collect-log": "Занесите находку в хронологию",
    "verify-source": "Оцените личность и надежность источника", "verify-origin": "Найдите наиболее раннюю доступную публикацию", "verify-reverse": "Выполните обратный поиск изображения или кадров", "verify-metadata": "Изучите метаданные, не считая их доказательством", "verify-time": "Проверьте дату, время и последовательность", "verify-place": "Проверьте место по независимым ориентирам", "verify-corroborate": "Найдите независимое подтверждение",
    "structure-nodes": "Создайте карточки важных субъектов и улик", "structure-edges": "Соедините подтвержденные и предполагаемые связи", "structure-timeline": "Расположите ключевые события по времени", "structure-map": "Отметьте проверенные места и перемещения", "structure-conflicts": "Фиксируйте противоречия, а не скрывайте их",
    "analyze-independent": "Отделите независимые источники от повторов", "analyze-alternatives": "Проверьте хотя бы одно альтернативное объяснение", "analyze-disconfirm": "Ищите данные, опровергающие основную версию", "analyze-confidence": "Назначьте уверенность каждому выводу", "analyze-gaps": "Перечислите неизвестное и пробелы в доказательствах",
    "report-findings": "Свяжите каждый вывод с подтверждающими материалами", "report-caveats": "Укажите ограничения и нерешенные противоречия", "report-privacy": "Удалите лишние персональные и чувствительные данные", "report-reproduce": "Проверьте, сможет ли другой аналитик повторить путь", "report-next": "Сформулируйте следующие вопросы или закройте расследование",
  };

  async function init() {
    if (!state.initialized) { cache(); bind(); state.initialized = true; }
    await load();
  }

  function cache() {
    ["investigation-checklist", "checklist-backend", "checklist-add", "checklist-percent", "checklist-count", "checklist-meter", "checklist-next", "checklist-next-title", "checklist-phases", "checklist-error", "checklist-retry", "checklist-dialog", "checklist-form", "checklist-dialog-title", "checklist-dialog-close", "checklist-dialog-cancel", "checklist-task-id", "checklist-task-phase", "checklist-task-title", "checklist-task-note", "checklist-task-status", "checklist-task-recommended", "checklist-task-delete"].forEach((id) => { el[id] = document.getElementById(id); });
  }

  function bind() {
    el["checklist-add"].addEventListener("click", openAdd);
    el["checklist-retry"].addEventListener("click", load);
    el["checklist-next"].addEventListener("click", revealRecommended);
    el["checklist-phases"].addEventListener("change", toggleTask);
    el["checklist-phases"].addEventListener("click", handleTaskAction);
    el["checklist-phases"].addEventListener("toggle", (event) => { if (event.target.open) state.openPhase = event.target.dataset.phaseId; }, true);
    el["checklist-form"].addEventListener("submit", saveTask);
    el["checklist-dialog-close"].addEventListener("click", closeDialog);
    el["checklist-dialog-cancel"].addEventListener("click", closeDialog);
    el["checklist-task-delete"].addEventListener("click", deleteTask);
    document.addEventListener("amber:case-switched", () => load());
    document.addEventListener("amber:case-created", () => load());
  }

  async function load() {
    try {
      const firstLoad = !state.snapshot;
      const caseResponse = await api("/api/case");
      state.caseID = caseResponse.id;
      el["investigation-checklist"].hidden = caseResponse.subject?.codename === "UNASSIGNED";
      if (el["investigation-checklist"].hidden) return;
      state.snapshot = await api("/api/checklist");
      el["checklist-error"].hidden = true; el["checklist-phases"].hidden = false;
      render();
      if (firstLoad) AmberMotion.revealList(el["checklist-phases"].querySelectorAll("details"), { limit: 4, axis: "x" });
    } catch (error) {
      el["checklist-error"].hidden = false; el["checklist-phases"].hidden = true; notify(`${I18n.t("checklist.error")} / ${error.message}`, true);
    }
  }

  function render() {
    if (!state.snapshot) return;
    const tasks = state.snapshot.phases.flatMap((phase) => phase.tasks);
    const resolved = tasks.filter((task) => task.status !== "pending").length;
    const percent = tasks.length ? Math.round(resolved * 100 / tasks.length) : 0;
    const recommended = recommendedTask(tasks);
    el["checklist-backend"].textContent = (state.snapshot.backend || "memory").toUpperCase();
    el["checklist-percent"].textContent = `${percent}%`; el["checklist-count"].textContent = `${resolved} / ${tasks.length}`; el["checklist-meter"].style.transform = `scaleX(${percent / 100})`;
    el["checklist-next"].disabled = !recommended; el["checklist-next"].dataset.taskId = recommended?.id || ""; el["checklist-next-title"].textContent = recommended ? taskTitle(recommended) : I18n.t("checklist.complete");
    if (!state.openPhase) state.openPhase = recommended?.phaseId || state.snapshot.phases[0]?.id || "";
    el["checklist-task-phase"].replaceChildren(...state.snapshot.phases.map((phase) => option(phase.id, phaseTitle(phase))));
    el["checklist-phases"].replaceChildren(...state.snapshot.phases.map((phase, index) => renderPhase(phase, index, recommended?.id)));
    I18n.apply(el["investigation-checklist"]);
  }

  function renderPhase(phase, index, recommendedID) {
    const details = document.createElement("details"); details.dataset.phaseId = phase.id; details.open = phase.id === state.openPhase;
    const summary = document.createElement("summary");
    const resolved = phase.tasks.filter((task) => task.status !== "pending").length;
    summary.append(text("span", "checklist-phase-code", String(index + 1).padStart(2, "0")), text("span", "checklist-phase-title", phaseTitle(phase)), text("span", "checklist-phase-count", `${resolved}/${phase.tasks.length}`));
    const list = document.createElement("div"); list.className = "checklist-tasks";
    list.replaceChildren(...phase.tasks.map((task) => renderTask(task, task.id === recommendedID)));
    details.append(summary, list); return details;
  }

  function renderTask(task, recommended) {
    const row = document.createElement("div"); row.className = "checklist-task"; row.dataset.taskId = task.id; row.dataset.status = task.status; row.dataset.recommended = String(recommended);
    const check = document.createElement("input"); check.type = "checkbox"; check.checked = task.status === "done"; check.dataset.toggleTask = task.id; check.setAttribute("aria-label", `${I18n.t("checklist.toggle")}: ${taskTitle(task)}`);
    const copy = text("span", "checklist-task-copy", taskTitle(task)); if (task.status === "skipped") copy.append(text("small", "checklist-task-status", I18n.t("checklist.skipped"))); if (task.note) copy.append(text("small", "", task.note));
    const action = text("button", "checklist-task-action", ">"); action.type = "button"; action.dataset.action = task.action || ""; action.disabled = !task.action; action.title = I18n.t("checklist.openWorkspace"); action.setAttribute("aria-label", I18n.t("checklist.openWorkspace"));
    const edit = text("button", "checklist-task-edit", "..."); edit.type = "button"; edit.dataset.editTask = task.id; edit.title = I18n.t("checklist.edit"); edit.setAttribute("aria-label", `${I18n.t("checklist.edit")}: ${taskTitle(task)}`);
    row.append(check, copy, action, edit); return row;
  }

  async function toggleTask(event) {
    const control = event.target.closest("input[data-toggle-task]"); if (!control) return;
    control.disabled = true;
    await mutate(`/api/checklist/tasks/${encodeURIComponent(control.dataset.toggleTask)}`, { method: "PUT", body: JSON.stringify({ caseId: state.caseID, status: control.checked ? "done" : "pending" }) });
  }

  function handleTaskAction(event) {
    const edit = event.target.closest("button[data-edit-task]"); if (edit) { openEdit(edit.dataset.editTask); return; }
    const action = event.target.closest("button[data-action]")?.dataset.action; if (!action) return;
    const views = { timeline: "timeline-view", map: "map-view", relationships: "relations-view", catalog: "catalog-view" };
    document.querySelector(`[data-work-view="${views[action]}"]`)?.click();
  }

  function revealRecommended() {
    const task = findTask(el["checklist-next"].dataset.taskId); if (!task) return;
    state.openPhase = task.phaseId; render();
    el["checklist-phases"].querySelector(`[data-task-id="${CSS.escape(task.id)}"]`)?.scrollIntoView({ behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth", block: "nearest" });
  }

  function openAdd() {
    el["checklist-form"].reset(); el["checklist-task-id"].value = ""; el["checklist-task-phase"].disabled = false; el["checklist-task-phase"].value = state.openPhase || state.snapshot.phases[0].id; el["checklist-task-status"].value = "pending"; el["checklist-task-delete"].hidden = true; el["checklist-dialog-title"].textContent = I18n.t("checklist.add"); el["checklist-dialog"].showModal(); el["checklist-task-title"].focus();
  }

  function openEdit(id) {
    const task = findTask(id); if (!task) return;
    el["checklist-task-id"].value = task.id; el["checklist-task-phase"].value = task.phaseId; el["checklist-task-phase"].disabled = true; el["checklist-task-title"].value = taskTitle(task); el["checklist-task-title"].dataset.originalTitle = taskTitle(task); el["checklist-task-note"].value = task.note || ""; el["checklist-task-status"].value = task.status; el["checklist-task-recommended"].checked = state.snapshot.recommendedTaskId === task.id; el["checklist-task-delete"].hidden = !task.custom; el["checklist-dialog-title"].textContent = I18n.t("checklist.edit"); el["checklist-dialog"].showModal(); el["checklist-task-title"].focus();
  }

  async function saveTask(event) {
    event.preventDefault();
    const id = el["checklist-task-id"].value, title = el["checklist-task-title"].value.trim();
    const body = id
      ? { caseId: state.caseID, note: el["checklist-task-note"].value.trim(), status: el["checklist-task-status"].value, recommended: el["checklist-task-recommended"].checked }
      : { caseId: state.caseID, phaseId: el["checklist-task-phase"].value, title, note: el["checklist-task-note"].value.trim() };
    if (id && title !== el["checklist-task-title"].dataset.originalTitle) body.title = title;
    const ok = id ? await mutate(`/api/checklist/tasks/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(body) }) : await mutate("/api/checklist/tasks", { method: "POST", body: JSON.stringify(body) });
    if (ok) closeDialog();
  }

  async function deleteTask() {
    const id = el["checklist-task-id"].value, task = findTask(id); if (!task?.custom || !confirm(`${I18n.t("checklist.deleteConfirm")}\n\n${taskTitle(task)}`)) return;
    const ok = await mutate(`/api/checklist/tasks/${encodeURIComponent(id)}`, { method: "DELETE", body: JSON.stringify({ caseId: state.caseID }) }); if (ok) closeDialog();
  }

  async function mutate(url, options) {
    try { const result = await api(url, options); if (result) state.snapshot = result; else await load(); render(); AmberMotion.pulse(el["checklist-percent"]); notify(I18n.t("checklist.saved")); return true; }
    catch (error) { notify(`${I18n.t("checklist.error")} / ${error.message}`, true); return false; }
  }

  async function api(url, options = {}) {
    const response = await fetch(url, { headers: { Accept: "application/json", ...(options.body ? { "Content-Type": "application/json" } : {}) }, ...options });
    if (response.status === 204) return null;
    const payload = await response.json(); if (!response.ok) throw new Error(payload.error || `API ${response.status}`); return payload;
  }

  function recommendedTask(tasks) { const pinned = tasks.find((task) => task.id === state.snapshot.recommendedTaskId && task.status === "pending"); return pinned || tasks.find((task) => task.status === "pending") || null; }
  function findTask(id) { return state.snapshot?.phases.flatMap((phase) => phase.tasks).find((task) => task.id === id); }
  function phaseTitle(phase) { return I18n.language === "ru" ? phaseRU[phase.id] || phase.title : phase.title; }
  function taskTitle(task) { return task.edited || task.custom || I18n.language !== "ru" ? task.title : taskRU[task.id] || task.title; }
  function text(tag, className, value) { const node = document.createElement(tag); if (className) node.className = className; node.textContent = value; return node; }
  function option(value, label) { const node = document.createElement("option"); node.value = value; node.textContent = label; return node; }
  function closeDialog() { if (el["checklist-dialog"].open) el["checklist-dialog"].close(); }
  function notify(message, error = false) { const output = document.getElementById("command-output"); if (output) AmberMotion.typeText(output, message, { tone: error ? "error" : "ok" }); }

  return { init, load, render };
})();
