"use strict";

window.AmberMap = (() => {
  const state = { map: null, mode: "select", markers: [], routes: [], layers: new Map(), routeLayers: new Map(), detailLayers: [], countryLabels: [], cityLabels: [], localBasemap: false, routeStart: null, selectedMarker: null, selectedRoute: null, loaded: false };
  const el = {};

  document.addEventListener("amber:case-switched", () => { if (state.map) refresh(); });

  function cache() {
    ["map-modes", "map-status", "map-hint", "marker-form", "marker-id", "marker-label", "marker-lat", "marker-lng", "marker-time", "marker-description", "marker-delete", "route-inspector", "route-label", "route-delete"].forEach((id) => { el[id] = document.getElementById(id); });
  }

  async function init() {
    if (state.map) return refresh();
    cache();
    const basemap = await fetch("/api/map/basemap").then((response) => response.json()).catch(() => ({ configured: false, minZoom: 2, maxZoom: 8 }));
    state.localBasemap = Boolean(basemap.configured);
    state.map = L.map("world-map", { attributionControl: false, zoomControl: true, minZoom: state.localBasemap ? basemap.minZoom : 2, maxZoom: state.localBasemap ? basemap.maxZoom : 8, worldCopyJump: true }).setView([24, 18], 2);
    createPane("localTiles", 100);
    createPane("countries", 200);
    createPane("urban", 210);
    createPane("hydro", 220);
    createPane("grid", 240);
    createPane("regions", 260);
    createPane("roads", 280);
    createPane("countryLabels", 300);
    createPane("cityLabels", 350);
    state.map.createPane("routes");
    state.map.getPane("routes").style.zIndex = 500;
    createPane("investigationMarkers", 600);
    const attribution = L.control.attribution({ prefix: false, position: "bottomleft" }).addTo(state.map);
    attribution.addAttribution(state.localBasemap ? "Local XYZ map pack" : "Natural Earth / local dataset");
    if (state.localBasemap) {
      L.tileLayer("/api/map/tiles/{z}/{x}/{y}", { pane: "localTiles", minZoom: basemap.minZoom, maxZoom: basemap.maxZoom, maxNativeZoom: basemap.maxZoom, noWrap: false, updateWhenIdle: true, keepBuffer: 2 }).addTo(state.map);
    }
    setStatus("LOADING LOCAL MAP");
    const [countries, cities] = await Promise.all([
      fetch("data/world.geojson").then((response) => response.json()),
      fetch("data/cities.geojson").then((response) => response.json()),
    ]);
    L.geoJSON(countries, { pane: "countries", renderer: L.canvas({ pane: "countries", padding: 0.5 }), style: { color: "#755022", weight: 1, opacity: state.localBasemap ? 0.35 : 0.9, fillColor: "#120d08", fillOpacity: state.localBasemap ? 0.06 : 1 }, interactive: false }).addTo(state.map);
    if (!state.localBasemap) {
      const [regions, urban, rivers, lakes, roads] = await Promise.all([
        fetch("data/regions.geojson").then((response) => response.json()),
        fetch("data/urban.geojson").then((response) => response.json()),
        fetch("data/rivers.geojson").then((response) => response.json()),
        fetch("data/lakes.geojson").then((response) => response.json()),
        fetch("data/roads.geojson").then((response) => response.json()),
      ]);
      buildDetailLayers({ regions, urban, rivers, lakes, roads });
    }
    drawGrid();
    if (!state.localBasemap) {
      buildCountryLabels(countries.features || []);
      buildCityLabels(cities.features || []);
    }
    updateGeographyLabels();
    bind();
    state.loaded = true;
    await refresh();
  }

  function bind() {
    el["map-modes"].addEventListener("click", (event) => {
      const button = event.target.closest("button[data-map-mode]");
      if (!button) return;
      setMode(button.dataset.mapMode);
    });
    state.map.on("click", (event) => {
      if (state.mode !== "marker") return;
      selectDraft(event.latlng.lat, event.latlng.lng);
    });
    state.map.on("zoomend", updateGeographyLabels);
    el["marker-form"].addEventListener("submit", saveMarker);
    el["marker-delete"].addEventListener("click", deleteMarker);
    el["route-delete"].addEventListener("click", deleteRoute);
  }

  function drawGrid() {
    const style = { pane: "grid", color: "#4a3218", weight: 1, opacity: 0.28, interactive: false };
    for (let lat = -60; lat <= 60; lat += 30) L.polyline([[-0 + lat, -180], [lat, 180]], style).addTo(state.map);
    for (let lng = -150; lng <= 180; lng += 30) L.polyline([[-85, lng], [85, lng]], style).addTo(state.map);
  }

  function createPane(name, zIndex) {
    state.map.createPane(name);
    state.map.getPane(name).style.zIndex = zIndex;
  }

  function buildCountryLabels(features) {
    state.countryLabels = features.flatMap((feature) => {
      const properties = feature.properties || {};
      const latitude = Number(properties.LABEL_Y);
      const longitude = Number(properties.LABEL_X);
      if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) return [];
      const name = properties.NAME || properties.ADMIN;
      const layer = L.marker([latitude, longitude], {
        pane: "countryLabels",
        interactive: false,
        icon: L.divIcon({ className: "amber-country-label", html: escapeHTML(name), iconSize: [1, 1], iconAnchor: [0, 0] }),
      });
      return [{ layer, minZoom: Math.max(2, Math.ceil(Number(properties.MIN_LABEL) || 2)) }];
    });
  }

  function buildCityLabels(features) {
    state.cityLabels = features.flatMap((feature) => {
      const properties = feature.properties || {};
      const coordinates = feature.geometry?.coordinates || [];
      const longitude = Number(coordinates[0]);
      const latitude = Number(coordinates[1]);
      if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) return [];
      const rank = Number(properties.scalerank) || 0;
      const minZoom = Math.max(2, Math.ceil(Number(properties.min_zoom) || cityMinimumZoom(rank)));
      const layer = L.marker([latitude, longitude], {
        pane: "cityLabels",
        interactive: false,
        icon: L.divIcon({ className: `amber-city-label${properties.adm0cap === 1 ? " capital" : ""}`, html: `<i></i><span>${escapeHTML(properties.name || properties.nameascii || "")}</span>`, iconSize: [110, 14], iconAnchor: [3, 7] }),
      });
      return [{ layer, rank, minZoom }];
    });
  }

  function buildDetailLayers(data) {
    const canvas = (pane) => L.canvas({ pane, padding: 0.5 });
    state.detailLayers = [
      { minZoom: 2, layer: L.geoJSON(data.lakes, { pane: "hydro", renderer: canvas("hydro"), interactive: false, style: { color: "#48655d", weight: 0.7, opacity: 0.85, fillColor: "#090e0d", fillOpacity: 1 } }) },
      { minZoom: 3, layer: L.geoJSON(data.rivers, { pane: "hydro", renderer: canvas("hydro"), interactive: false, style: { color: "#48655d", weight: 0.65, opacity: 0.75 } }) },
      { minZoom: 3, layer: L.geoJSON(data.regions, { pane: "regions", renderer: canvas("regions"), interactive: false, style: { color: "#60421f", weight: 0.55, opacity: 0.65, dashArray: "3 4" } }) },
      { minZoom: 4, layer: L.geoJSON(data.urban, { pane: "urban", renderer: canvas("urban"), interactive: false, style: { color: "#754b1e", weight: 0.5, opacity: 0.65, fillColor: "#2d1b0b", fillOpacity: 0.85 } }) },
      ...[3, 4, 5].map((rank) => ({ minZoom: rank, layer: L.geoJSON(data.roads, { pane: "roads", renderer: canvas("roads"), interactive: false, filter: (feature) => Number(feature.properties?.scalerank) === rank, style: roadStyle(rank) }) })),
    ];
  }

  function updateGeographyLabels() {
    const zoom = state.map.getZoom();
    state.detailLayers.forEach(({ layer, minZoom }) => toggleLayer(layer, zoom >= minZoom));
    state.countryLabels.forEach(({ layer, minZoom }) => toggleLayer(layer, zoom >= minZoom && zoom <= 6));
    state.cityLabels.forEach(({ layer, minZoom }) => toggleLayer(layer, zoom >= minZoom));
  }

  function cityMinimumZoom(rank) { return ({ 0: 2, 1: 3, 2: 4, 3: 5, 4: 6, 5: 7, 6: 7, 7: 8, 8: 8 })[rank] ?? 8; }
  function roadStyle(rank) { return { color: rank === 3 ? "#b37832" : rank === 4 ? "#895823" : "#654119", weight: rank === 3 ? 1.2 : rank === 4 ? 0.8 : 0.55, opacity: rank === 3 ? 0.8 : 0.62 }; }

  function toggleLayer(layer, visible) {
    const present = state.map.hasLayer(layer);
    if (visible && !present) layer.addTo(state.map);
    if (!visible && present) layer.remove();
  }

  async function refresh() {
    setStatus("SYNCING");
    try {
      const snapshot = await request("/api/map");
      state.markers = snapshot.markers || [];
      state.routes = snapshot.routes || [];
      render();
      const base = state.localBasemap ? "LOCAL DETAIL PACK" : `${state.cityLabels.length} CITIES`;
      setStatus(`${snapshot.backend.toUpperCase()} / ${base} / ${state.markers.length} MARKERS / ${state.routes.length} ROUTES`);
    } catch (error) {
      setStatus(`ERROR / ${error.message}`, true);
    }
  }

  function render() {
    state.layers.forEach((layer) => layer.remove());
    state.routeLayers.forEach((layer) => layer.remove());
    state.layers.clear();
    state.routeLayers.clear();
    const byID = new Map(state.markers.map((marker) => [marker.id, marker]));
    state.routes.forEach((route) => {
      const from = byID.get(route.fromMarkerId);
      const to = byID.get(route.toMarkerId);
      if (!from || !to) return;
      const layer = L.polyline([[from.latitude, from.longitude], [to.latitude, to.longitude]], { pane: "routes", color: "#f2ad3e", weight: 2, opacity: 0.9, dashArray: "8 6" }).addTo(state.map);
      const routeLabel = route.label || `${from.label} -> ${to.label}`;
      layer.bindTooltip(escapeHTML(routeLabel), { pane: "routes", permanent: true, direction: "center", className: "amber-route-label" });
      layer.on("click", () => selectRoute(route));
      state.routeLayers.set(route.id, layer);
    });
    state.markers.forEach((marker, index) => {
      const icon = L.divIcon({ className: "amber-map-icon", html: `<span>${String(index + 1).padStart(2, "0")}</span>`, iconSize: [28, 28], iconAnchor: [14, 14] });
      const layer = L.marker([marker.latitude, marker.longitude], { pane: "investigationMarkers", icon, draggable: true, title: marker.label }).addTo(state.map);
      layer.bindTooltip(escapeHTML(marker.label), { pane: "investigationMarkers", permanent: true, direction: "right", offset: [15, 0], className: "amber-object-label" });
      layer.on("click", () => markerClicked(marker));
      layer.on("dragend", async () => {
        const position = layer.getLatLng();
        marker.latitude = round(position.lat);
        marker.longitude = round(position.lng);
        await persistMarker(marker);
      });
      state.layers.set(marker.id, layer);
    });
  }

  function markerClicked(marker) {
    if (state.mode === "route") {
      if (!state.routeStart) {
        state.routeStart = marker.id;
        setStatus(`ROUTE START / ${marker.label}`);
        state.layers.get(marker.id)?.getElement()?.classList.add("route-origin");
        return;
      }
      if (state.routeStart === marker.id) return;
      createRoute(state.routeStart, marker.id);
      return;
    }
    selectMarker(marker);
  }

  function setMode(mode) {
    state.mode = mode;
    state.routeStart = null;
    document.querySelectorAll("[data-map-mode]").forEach((button) => button.classList.toggle("active", button.dataset.mapMode === mode));
    document.getElementById("world-map").classList.toggle("placing", mode === "marker");
    setStatus(mode === "marker" ? "CLICK MAP TO PLACE" : mode === "route" ? "SELECT FIRST MARKER" : "READY");
  }

  function selectDraft(latitude, longitude) {
    state.selectedMarker = null;
    el["marker-form"].reset();
    el["marker-id"].value = "";
    el["marker-lat"].value = round(latitude);
    el["marker-lng"].value = round(longitude);
    el["marker-time"].value = localDateTime(new Date());
    el["marker-delete"].hidden = true;
    showMarkerForm();
    el["marker-label"].focus();
  }

  function selectMarker(marker) {
    state.selectedMarker = marker;
    el["marker-id"].value = marker.id;
    el["marker-label"].value = marker.label;
    el["marker-lat"].value = marker.latitude;
    el["marker-lng"].value = marker.longitude;
    el["marker-time"].value = marker.occurredAt ? localDateTime(new Date(marker.occurredAt)) : "";
    el["marker-description"].value = marker.description || "";
    el["marker-delete"].hidden = false;
    showMarkerForm();
    state.map.panTo([marker.latitude, marker.longitude]);
  }

  function showMarkerForm() {
    el["map-hint"].hidden = true;
    el["route-inspector"].hidden = true;
    el["marker-form"].hidden = false;
  }

  function selectRoute(route) {
    state.selectedRoute = route;
    const from = state.markers.find((marker) => marker.id === route.fromMarkerId);
    const to = state.markers.find((marker) => marker.id === route.toMarkerId);
    el["marker-form"].hidden = true;
    el["map-hint"].hidden = true;
    el["route-inspector"].hidden = false;
    el["route-label"].textContent = route.label || `${from?.label || route.fromMarkerId} -> ${to?.label || route.toMarkerId}`;
  }

  async function saveMarker(event) {
    event.preventDefault();
    const marker = formMarker();
    try {
      const saved = marker.id ? await request(`/api/map/markers/${encodeURIComponent(marker.id)}`, { method: "PUT", body: JSON.stringify(marker) }) : await request("/api/map/markers", { method: "POST", body: JSON.stringify(marker) });
      state.selectedMarker = saved;
      setMode("select");
      await refresh();
      selectMarker(saved);
    } catch (error) { setStatus(`ERROR / ${error.message}`, true); }
  }

  async function persistMarker(marker) {
    try {
      await request(`/api/map/markers/${encodeURIComponent(marker.id)}`, { method: "PUT", body: JSON.stringify(marker) });
      setStatus(`MOVED / ${marker.label}`);
      await refresh();
      selectMarker(marker);
    } catch (error) { setStatus(`ERROR / ${error.message}`, true); await refresh(); }
  }

  async function deleteMarker() {
    const id = el["marker-id"].value;
    if (!id || !window.confirm("Delete marker and connected routes?")) return;
    try { await request(`/api/map/markers/${encodeURIComponent(id)}`, { method: "DELETE" }); clearInspector(); await refresh(); } catch (error) { setStatus(`ERROR / ${error.message}`, true); }
  }

  async function createRoute(fromMarkerId, toMarkerId) {
    try {
      await request("/api/map/routes", { method: "POST", body: JSON.stringify({ fromMarkerId, toMarkerId, label: "", startedAt: "", endedAt: "", notes: "" }) });
      state.routeStart = null;
      setMode("select");
      await refresh();
    } catch (error) { setStatus(`ERROR / ${error.message}`, true); }
  }

  async function deleteRoute() {
    if (!state.selectedRoute) return;
    try { await request(`/api/map/routes/${encodeURIComponent(state.selectedRoute.id)}`, { method: "DELETE" }); clearInspector(); await refresh(); } catch (error) { setStatus(`ERROR / ${error.message}`, true); }
  }

  function formMarker() {
    return { id: el["marker-id"].value, label: el["marker-label"].value.trim(), latitude: Number(el["marker-lat"].value), longitude: Number(el["marker-lng"].value), occurredAt: el["marker-time"].value ? new Date(el["marker-time"].value).toISOString() : "", description: el["marker-description"].value.trim(), eventIds: state.selectedMarker?.eventIds || [] };
  }

  function clearInspector() {
    state.selectedMarker = null;
    state.selectedRoute = null;
    el["marker-form"].hidden = true;
    el["route-inspector"].hidden = true;
    el["map-hint"].hidden = false;
  }

  async function request(url, options = {}) {
    const response = await fetch(url, { ...options, headers: { Accept: "application/json", "Content-Type": "application/json" } });
    if (response.status === 204) return null;
    const payload = await response.json();
    if (!response.ok) throw new Error(payload.error || `API ${response.status}`);
    return payload;
  }

  function setStatus(message, error = false) { el["map-status"].textContent = message; el["map-status"].classList.toggle("error", error); }
  function round(value) { return Math.round(value * 100000) / 100000; }
  function localDateTime(date) { const shifted = new Date(date.getTime() - date.getTimezoneOffset() * 60000); return shifted.toISOString().slice(0, 16); }
  function escapeHTML(value) { const element = document.createElement("span"); element.textContent = String(value || ""); return element.innerHTML; }

  return { init, refresh };
})();
