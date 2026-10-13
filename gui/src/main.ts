// The settings window. It renders the latest Snapshot (`jingle status`)
// plus `jingle schema`, and every control runs one jingle command. Nothing
// here validates or stores config: jingle does, and its error messages are
// shown as they come.

import { listen } from "@tauri-apps/api/event";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { open } from "@tauri-apps/plugin-dialog";

import { api, errorMessage, SCHEMA_VERSION, type Problem, type RepoReport, type Schema, type Snapshot, type Version } from "./api";
import { h, icons, toggle } from "./dom";
import "./style.css";

interface State {
  snap: Snapshot | null;
  schema: Schema | null;
  version: Version | null;
  /** A file picked for an event, waiting to be previewed and added. */
  pending: Record<string, string | undefined>;
  /** Built-in chosen in each event's "add" picker. */
  builtinChoice: Record<string, string | undefined>;
  repo: RepoReport | null;
  confirmUninstall: boolean;
  busy: boolean;
}

const state: State = {
  snap: null,
  schema: null,
  version: null,
  pending: {},
  builtinChoice: {},
  repo: null,
  confirmUninstall: false,
  busy: false,
};

const root = document.getElementById("app")!;
let lastRendered = "";
let dragging = false;
let renderQueued = false;

// ---- actions --------------------------------------------------------------

/** Runs a jingle command, showing its error if it fails. */
async function run(action: () => Promise<unknown>): Promise<boolean> {
  try {
    await action();
    return true;
  } catch (e) {
    flash(errorMessage(e));
    return false;
  }
}

async function loadMeta() {
  [state.schema, state.version] = await Promise.all([api.schema().catch(() => null), api.version().catch(() => null)]);
}

async function boot() {
  await listen<Snapshot>("state-changed", (e) => setSnapshot(e.payload));
  setSnapshot(await api.snapshot());
  if (state.snap?.status) await loadMeta();
  render(true);
}

async function relocate() {
  state.busy = true;
  render(true);
  const snap = await api.relocate();
  if (snap.status) await loadMeta();
  state.busy = false;
  setSnapshot(snap);
  render(true);
}

function setSnapshot(snap: Snapshot) {
  const hadStatus = Boolean(state.snap?.status);
  state.snap = snap;
  // jingle just appeared (installed with the window open): fetch its schema.
  if (!hadStatus && snap.status && !state.schema) {
    void loadMeta().then(() => render(true));
  }
  render();
}

// ---- rendering ------------------------------------------------------------

/** Re-renders unless nothing changed; never mid-drag on the volume slider. */
function render(force = false) {
  if (dragging) {
    renderQueued = true;
    return;
  }
  const key = JSON.stringify([state.snap, state.schema, state.version, state.pending, state.builtinChoice, state.repo, state.confirmUninstall, state.busy]);
  if (!force && key === lastRendered) return;
  lastRendered = key;
  const scroll = document.scrollingElement?.scrollTop ?? 0;
  root.replaceChildren(view());
  if (document.scrollingElement) document.scrollingElement.scrollTop = scroll;
}

function view(): HTMLElement {
  const snap = state.snap;
  if (!snap) return h("main", { class: "loading" }, "Loading…");
  if (!snap.status) return notFoundView(snap);
  return h("main", {}, header(snap), setupCard(snap), problemsCard(snap.status.problems), eventsSection(snap), generalCard(snap), repoCard(), footer(snap));
}

function notFoundView(snap: Snapshot): HTMLElement {
  const missing = !snap.jingle;
  return h(
    "main",
    { class: "empty" },
    h("span", { class: "empty-icon" }, icons.logo()),
    h("h1", {}, missing ? "Install the jingle command-line tool" : "jingle isn't responding"),
    missing
      ? h("p", {}, "This app is a front end for the jingle CLI, which does the actual work. Install it with one of these, then try again:")
      : h("p", {}, snap.error?.message ?? "Unknown error."),
    missing && h("pre", {}, "brew install OPDhaker/tap/jingle"),
    missing && h("pre", {}, "curl -fsSL https://raw.githubusercontent.com/OPDhaker/jingle/main/install.sh | sh"),
    missing && snap.error && h("p", { class: "muted small" }, snap.error.message),
    h("button", { class: "primary", disabled: state.busy, onclick: () => void relocate() }, state.busy ? "Looking…" : "Try again"),
  );
}

function header(snap: Snapshot): HTMLElement {
  const s = snap.status!;
  const on = s.installed && s.config.enabled === true;
  const hasErrors = s.problems.some((p) => p.severity === "error");
  const tone = !s.installed ? "off" : !on ? "off" : hasErrors ? "bad" : "ok";
  return h(
    "header",
    {},
    h("span", { class: "logo" }, icons.logo()),
    h("div", { class: "title" }, h("h1", {}, "Jingle"), h("span", { class: `pill ${tone}` }, snap.headline)),
    s.installed && toggle("Sounds on", s.config.enabled === true, (v) => void run(() => api.configSet("enabled", [String(v)]))),
  );
}

function setupCard(snap: Snapshot): HTMLElement | null {
  const s = snap.status!;
  if (!s.installed) {
    return h(
      "section",
      { class: "card hero" },
      h("h2", {}, "Turn jingle on"),
      h(
        "p",
        {},
        "Installing points git's global ",
        h("code", {}, "core.hooksPath"),
        " at jingle, so every repo on this computer plays a sound. Your existing hooks keep running, and uninstalling puts your git config back exactly as it was.",
      ),
      h("button", { class: "primary", disabled: state.busy, onclick: () => void busy(() => api.install()) }, "Install"),
    );
  }
  return null;
}

function problemsCard(problems: Problem[]): HTMLElement | null {
  // not_installed is what the install card is for.
  const shown = problems.filter((p) => p.code !== "not_installed");
  if (shown.length === 0) return null;
  // Same counting as the headline: errors are problems, warnings are not.
  const errors = shown.filter((p) => p.severity === "error").length;
  const warnings = shown.length - errors;
  const title = [errors && plural(errors, "problem"), warnings && plural(warnings, "warning")].filter(Boolean).join(", ");
  return h("section", { class: "card problems" }, h("h2", {}, title), problemList(shown));
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

function problemList(problems: Problem[]): HTMLElement {
  return h(
    "ul",
    { class: "problem-list" },
    problems.map((p) =>
      h(
        "li",
        { class: p.severity },
        h("span", { class: "dot", title: p.severity }),
        h("div", {}, h("p", {}, p.message), p.fix && h("code", { class: "fix" }, p.fix)),
      ),
    ),
  );
}

const eventLabels: Record<string, string> = { commit: "Commit", push: "Push", merge: "Merge" };
const eventHints: Record<string, string> = {
  commit: "Plays after every commit. Rebases and cherry-picks stay quiet.",
  push: "Plays when a push goes through.",
  merge: "Plays for real merge commits, not fast-forwards.",
};

function eventsSection(snap: Snapshot): HTMLElement {
  const events = state.schema?.events ?? ["commit", "push", "merge"];
  return h("section", { class: "events" }, h("h2", { class: "section-title" }, "Sounds"), events.map((ev) => eventCard(snap, ev)));
}

function eventCard(snap: Snapshot, ev: string): HTMLElement {
  const cfg = snap.status!.config;
  const enabled = cfg[`${ev}.enabled`] === true;
  const sounds = (cfg[`${ev}.sounds`] as string[] | undefined) ?? [];
  const muted = cfg.enabled !== true || !snap.status!.installed;
  return h(
    "div",
    { class: "card event" + (enabled ? "" : " is-off") + (muted ? " is-muted" : "") },
    h(
      "div",
      { class: "row head" },
      h("div", {}, h("h3", {}, eventLabels[ev] ?? ev), h("p", { class: "muted small" }, eventHints[ev] ?? "")),
      toggle(`${eventLabels[ev] ?? ev} sound`, enabled, (v) => void run(() => api.configSet(`${ev}.enabled`, [String(v)]))),
    ),
    h(
      "ul",
      { class: "sounds" },
      sounds.length === 0 && h("li", { class: "muted small" }, "No sounds. Add one below."),
      sounds.map((entry) => soundRow(ev, entry)),
      pendingRow(ev),
    ),
    addRow(ev, sounds),
    ev === "push" && pushWhen(cfg["push.when"] as string),
  );
}

function soundLabel(entry: string, ev: string): { name: string; tag: string; title: string } {
  if (entry.startsWith("builtin:")) {
    const name = entry.slice("builtin:".length);
    const doc = state.schema?.builtin_sounds.find((b) => b.name === entry)?.doc ?? "";
    return { name: name.charAt(0).toUpperCase() + name.slice(1), tag: "built-in", title: doc };
  }
  let base = entry.split("/").pop() ?? entry;
  if (base.startsWith(`${ev}-`)) base = base.slice(ev.length + 1);
  return { name: base, tag: "file", title: entry };
}

function soundRow(ev: string, entry: string): HTMLElement {
  const l = soundLabel(entry, ev);
  return h(
    "li",
    { class: "sound" },
    iconButton("Play", icons.play(), () => run(() => api.play(ev, entry))),
    h("span", { class: "name", title: l.title }, l.name),
    h("span", { class: "tag" }, l.tag),
    iconButton(`Remove ${l.name}`, icons.remove(), () => run(() => api.configRemove(`${ev}.sounds`, [entry])), "danger"),
  );
}

function pendingRow(ev: string): HTMLElement | null {
  const file = state.pending[ev];
  if (!file) return null;
  const name = file.split("/").pop() ?? file;
  return h(
    "li",
    { class: "sound pending" },
    iconButton("Preview", icons.play(), () => run(() => api.preview(file))),
    h("span", { class: "name", title: file }, name),
    h(
      "button",
      {
        class: "small primary",
        onclick: async () => {
          if (await run(() => api.configAdd(`${ev}.sounds`, [file]))) {
            state.pending[ev] = undefined;
            render();
          }
        },
      },
      "Add",
    ),
    h(
      "button",
      {
        class: "small",
        onclick: () => {
          state.pending[ev] = undefined;
          render();
        },
      },
      "Cancel",
    ),
  );
}

function addRow(ev: string, sounds: string[]): HTMLElement {
  const builtins = (state.schema?.builtin_sounds ?? []).filter((b) => !sounds.includes(b.name));
  let choice = state.builtinChoice[ev];
  if (!choice || !builtins.some((b) => b.name === choice)) choice = builtins[0]?.name;
  return h(
    "div",
    { class: "row add" },
    builtins.length > 0 &&
      h(
        "div",
        { class: "builtin-picker" },
        h(
          "select",
          {
            "aria-label": "Built-in sound",
            onchange: (e: Event) => {
              state.builtinChoice[ev] = (e.target as HTMLSelectElement).value;
              render();
            },
          },
          builtins.map((b) => h("option", { value: b.name, selected: b.name === choice }, `${soundLabel(b.name, ev).name}: ${b.doc}`)),
        ),
        iconButton("Preview", icons.play(), () => run(() => api.preview(choice!))),
        h("button", { class: "small", onclick: () => void run(() => api.configAdd(`${ev}.sounds`, [choice!])) }, "Add"),
      ),
    h("button", { class: "small", onclick: () => void pickFile(ev) }, icons.plus(), "Add file…"),
  );
}

async function pickFile(ev: string) {
  const exts = (state.schema?.sound_types ?? []).map((e) => e.replace(/^\./, ""));
  const file = await open({
    multiple: false,
    directory: false,
    title: `Choose a ${ev} sound`,
    filters: exts.length ? [{ name: "Audio", extensions: exts }] : undefined,
  });
  if (typeof file === "string") {
    state.pending[ev] = file;
    render();
  }
}

function pushWhen(value: string): HTMLElement {
  const key = state.schema?.config_keys.find((k) => k.key === "push.when");
  const values = key?.values ?? [value];
  const labels: Record<string, string> = { success: "when the push succeeds", attempt: "when the push starts" };
  return h(
    "label",
    { class: "row field" },
    h("span", {}, "Play"),
    h(
      "select",
      { onchange: (e: Event) => void run(() => api.configSet("push.when", [(e.target as HTMLSelectElement).value])) },
      values.map((v) => h("option", { value: v, selected: v === value }, labels[v] ?? v)),
    ),
  );
}

function generalCard(snap: Snapshot): HTMLElement {
  const s = snap.status!;
  const keys = state.schema?.config_keys ?? [];
  const vol = keys.find((k) => k.key === "volume");
  const cd = keys.find((k) => k.key === "cooldown");
  const volume = Number(s.config.volume);
  const label = h("output", {}, `${volume}%`);
  return h(
    "section",
    { class: "card general" },
    h("h2", {}, "General"),
    h(
      "label",
      { class: "row field" },
      h("span", {}, "Volume"),
      h("input", {
        type: "range",
        min: vol?.min ?? 0,
        max: vol?.max ?? 100,
        step: 5,
        value: volume,
        onpointerdown: () => (dragging = true),
        oninput: (e: Event) => (label.textContent = `${(e.target as HTMLInputElement).value}%`),
        onchange: (e: Event) => {
          endDrag();
          void run(() => api.configSet("volume", [(e.target as HTMLInputElement).value]));
        },
      }),
      label,
    ),
    h(
      "label",
      { class: "row field" },
      h("span", { title: cd?.doc ?? "" }, "Quiet time between sounds"),
      h("input", {
        type: "number",
        min: cd?.min ?? 0,
        max: cd?.max,
        value: Number(s.config.cooldown),
        class: "num",
        onchange: (e: Event) => void run(() => api.configSet("cooldown", [(e.target as HTMLInputElement).value])),
      }),
      h("span", { class: "muted" }, "seconds"),
    ),
  );
}

function endDrag() {
  dragging = false;
  if (renderQueued) {
    renderQueued = false;
    render();
  }
}

function repoCard(): HTMLElement {
  const r = state.repo;
  const known = new Set(state.snap?.status?.problems.map((p) => p.code));
  const own = r ? r.problems.filter((p) => !known.has(p.code)) : [];
  return h(
    "section",
    { class: "card" },
    h(
      "div",
      { class: "row head" },
      h("div", {}, h("h2", {}, "Check a repo"), h("p", { class: "muted small" }, "Some tools, like husky, stop jingle from running in a repo.")),
      h("button", { class: "small", onclick: () => void pickRepo() }, "Choose folder…"),
    ),
    r &&
      h(
        "div",
        { class: "repo-result" },
        h("p", { class: "path" }, r.repo.path),
        own.length === 0 ? h("p", { class: "good" }, "jingle plays in this repo.") : problemList(own),
      ),
  );
}

async function pickRepo() {
  const dir = await open({ directory: true, multiple: false, title: "Choose a git repository" });
  if (typeof dir !== "string") return;
  try {
    state.repo = await api.doctorRepo(dir);
    render();
  } catch (e) {
    state.repo = null;
    flash(errorMessage(e));
    render();
  }
}

function footer(snap: Snapshot): HTMLElement {
  const s = snap.status!;
  const v = state.version;
  const mismatch = v && v.schema_version !== SCHEMA_VERSION;
  return h(
    "footer",
    {},
    mismatch && h("p", { class: "warn small" }, `This app expects jingle's JSON schema ${SCHEMA_VERSION}, but jingle reports ${v.schema_version}. Update the app or jingle.`),
    h("p", { class: "muted small" }, `jingle ${v?.version ?? ""} · ${snap.jingle?.bin ?? ""}`, s.player ? ` · player: ${s.player.name}` : ""),
    s.installed &&
      h(
        "div",
        { class: "row" },
        state.confirmUninstall
          ? [
              h("span", { class: "small" }, "Remove jingle's hooks and restore your git config?"),
              h("button", { class: "small danger", onclick: () => void busy(() => api.uninstall()).then(() => (state.confirmUninstall = false)) }, "Uninstall"),
              h("button", { class: "small", onclick: () => ((state.confirmUninstall = false), render()) }, "Cancel"),
            ]
          : h("button", { class: "small link", onclick: () => ((state.confirmUninstall = true), render()) }, "Uninstall…"),
      ),
  );
}

function iconButton(label: string, icon: SVGElement, onClick: () => unknown, cls = ""): HTMLElement {
  return h("button", { class: `icon ${cls}`, title: label, "aria-label": label, onclick: () => void onClick() }, icon);
}

async function busy(action: () => Promise<unknown>) {
  state.busy = true;
  render();
  await run(action);
  state.busy = false;
  render(true);
}

// ---- messages ---------------------------------------------------------------

let flashTimer: number | undefined;

function flash(message: string) {
  let el = document.getElementById("flash");
  if (!el) {
    el = h("div", { id: "flash", role: "alert" });
    document.body.append(el);
  }
  el.textContent = message;
  el.classList.add("show");
  window.clearTimeout(flashTimer);
  flashTimer = window.setTimeout(() => el!.classList.remove("show"), 6000);
}

// A menu-bar app has no menu, so Cmd+W has to be handled here.
window.addEventListener("keydown", (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key === "w") {
    e.preventDefault();
    void getCurrentWindow().hide();
  }
});
window.addEventListener("pointerup", () => dragging && window.setTimeout(endDrag, 0));

void boot();
