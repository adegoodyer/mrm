// mrm web UI: actions, live updates (server-sent events), filter as you type,
// and the per-browser settings. Every POST carries the session token in a
// header; the server refuses POSTs without it.
"use strict";
(function () {
  const token = document.querySelector('meta[name="mrm-token"]').content;
  const page = document.body.dataset.page;
  const repoID = document.body.dataset.repo || "";
  let toastTimer = null;

  function toast(msg, warn) {
    let el = document.getElementById("toast");
    if (!el) {
      el = document.createElement("div");
      el.id = "toast";
      el.setAttribute("role", "status");
      document.body.appendChild(el);
    }
    el.className = "toast" + (warn ? " warn" : "");
    el.textContent = msg;
    el.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.hidden = true; }, warn ? 8000 : 5000);
  }

  async function post(action, fields) {
    try {
      const r = await fetch("/api/" + action, {
        method: "POST",
        headers: { "X-MRM-Token": token, "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams(fields),
      });
      if (!r.ok) toast((await r.text()).trim() || r.statusText, true);
    } catch (e) {
      toast("mrm web is not reachable - is it still running?", true);
    }
  }

  // Filter.
  const q = document.getElementById("q");
  let filterTimer = null;
  async function refreshRows() {
    const rows = document.getElementById("rows");
    if (!rows) return;
    const v = q ? q.value.trim() : "";
    const r = await fetch("/rows?q=" + encodeURIComponent(v));
    const box = q && q.closest(".query");
    if (r.status === 400) {
      if (box) { box.classList.add("bad"); box.title = (await r.text()).trim(); }
      return;
    }
    if (box) { box.classList.remove("bad"); box.title = ""; }
    rows.innerHTML = await r.text();
    history.replaceState(null, "", v ? "?q=" + encodeURIComponent(v) : location.pathname);
    syncChips();
  }
  function terms() { return q ? q.value.split(/\s+/).filter(Boolean) : []; }
  function syncChips() {
    const t = terms();
    document.querySelectorAll(".chip").forEach(c => c.setAttribute("aria-pressed", String(t.includes(c.dataset.chip))));
  }
  if (q) {
    q.addEventListener("input", () => { clearTimeout(filterTimer); filterTimer = setTimeout(refreshRows, 150); });
    syncChips();
  }

  // Settings.
  function setTextSize(px) {
    document.documentElement.style.fontSize = px + "px";
    try { localStorage.setItem("mrm.textSize", String(px)); } catch (e) {}
  }
  function setTheme(t) {
    if (t === "system") document.documentElement.removeAttribute("data-theme");
    else document.documentElement.setAttribute("data-theme", t);
    try { if (t === "system") localStorage.removeItem("mrm.theme"); else localStorage.setItem("mrm.theme", t); } catch (e) {}
  }
  function markSettings() {
    const size = parseInt(document.documentElement.style.fontSize, 10);
    document.querySelectorAll("[data-size]").forEach(b => b.setAttribute("aria-pressed", String(Number(b.dataset.size) === size)));
    const theme = document.documentElement.getAttribute("data-theme") || "system";
    document.querySelectorAll("[data-theme-pick]").forEach(b => b.setAttribute("aria-pressed", String(b.dataset.themePick === theme)));
  }
  markSettings();

  document.addEventListener("click", e => {
    const t = e.target.closest("button");
    if (!t) return;
    if (t.dataset.action) {
      const scope = t.dataset.scope || "";
      post(t.dataset.action, {
        ws: t.dataset.ws || "",
        repo: t.dataset.repo || "",
        q: scope === "shown" ? (q ? q.value.trim() : "") : "",
        scope: scope,
      });
    } else if (t.dataset.chip) {
      const parts = terms();
      const i = parts.indexOf(t.dataset.chip);
      if (i >= 0) parts.splice(i, 1); else parts.push(t.dataset.chip);
      q.value = parts.join(" ");
      refreshRows();
    } else if (t.dataset.size) {
      setTextSize(Number(t.dataset.size));
      markSettings();
    } else if (t.dataset.themePick) {
      setTheme(t.dataset.themePick);
      markSettings();
    }
  });

  // Keys: / filters, esc clears or leaves the filter, ? lists the keys.
  const keys = [
    ["/", "filter repos (from any page: back to the overview)"],
    ["esc", "clear the filter, or leave it; close this list"],
    ["?", "show or hide these keys"],
  ];
  let keysBox = null;
  function toggleKeys() {
    if (!keysBox) {
      keysBox = document.createElement("dialog");
      keysBox.className = "keys";
      keysBox.setAttribute("aria-label", "Keys");
      const h = document.createElement("h2");
      h.textContent = "Keys";
      const list = document.createElement("div");
      list.className = "keylist";
      for (const [k, what] of keys) {
        const kbd = document.createElement("kbd");
        kbd.textContent = k;
        const desc = document.createElement("span");
        desc.textContent = what;
        list.append(kbd, desc);
      }
      keysBox.append(h, list);
      keysBox.addEventListener("click", e => { if (e.target === keysBox) keysBox.close(); });
      document.body.appendChild(keysBox);
    }
    if (keysBox.open) keysBox.close(); else keysBox.showModal();
  }
  function typing(el) {
    return el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName));
  }
  document.addEventListener("keydown", e => {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.target === q && e.key === "Escape") {
      if (q.value) { q.value = ""; refreshRows(); } else q.blur();
      e.preventDefault();
      return;
    }
    if (typing(e.target)) return;
    if (e.key === "/") {
      e.preventDefault();
      if (q) { q.focus(); q.select(); } else location.href = "/#filter";
    } else if (e.key === "?") {
      e.preventDefault();
      toggleKeys();
    }
  });
  if (q && location.hash === "#filter") {
    history.replaceState(null, "", location.pathname + location.search);
    q.focus();
  }

  // Live updates.
  function replace(id, html) {
    const el = document.getElementById(id);
    if (el && html) el.outerHTML = html;
  }
  let touchedRepo = false;
  const es = new EventSource("/events");
  es.addEventListener("row", e => {
    const d = JSON.parse(e.data);
    replace("row-" + d.id, d.row);
    replace("card-" + d.id, d.card);
    if (d.id === repoID) touchedRepo = true;
  });
  es.addEventListener("panels", e => {
    const d = JSON.parse(e.data);
    for (const [id, html] of Object.entries(d)) {
      const el = document.getElementById(id);
      if (el) el.innerHTML = html;
    }
  });
  es.addEventListener("busy", e => {
    const d = JSON.parse(e.data);
    const el = document.getElementById("busy");
    if (el) { el.textContent = d.text; el.classList.toggle("on", d.busy); }
    document.querySelectorAll("button[data-action]").forEach(b => { b.disabled = d.busy; });
    if (!d.busy && touchedRepo) location.reload();
    if (!d.busy && d.finished && page === "activity") location.reload();
  });
  es.addEventListener("toast", e => { const d = JSON.parse(e.data); toast(d.msg, d.warn); });
  es.addEventListener("reload", () => { refreshRows(); });
})();
